package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ resource.Resource                = (*FloatingIPResource)(nil)
	_ resource.ResourceWithConfigure   = (*FloatingIPResource)(nil)
	_ resource.ResourceWithImportState = (*FloatingIPResource)(nil)
)

// FloatingIPResource implements `viettelidc_floating_ip`.
//
// Create allocates a new Floating IP from CSA, then immediately associates it
// with the specified VM instance and NIC. Delete disassociates (the API has no
// separate delete endpoint — disassociate releases the IP back to the pool).
type FloatingIPResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// FloatingIPResourceModel mirrors the resource schema for State/Plan/Config marshalling.
type FloatingIPResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	PublicIP           types.String `tfsdk:"public_ip"`
	InstanceID         types.String `tfsdk:"instance_id"`
	NetworkInterfaceID types.String `tfsdk:"network_interface_id"`
	VpcID              types.String `tfsdk:"vpc_id"`
}

// NewFloatingIPResource constructs the resource (registered in iac/provider.go).
func NewFloatingIPResource() resource.Resource { return &FloatingIPResource{} }

func (r *FloatingIPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_floating_ip"
}

func (r *FloatingIPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Allocates a ViettelIDC Floating IP and associates it with a VM instance and NIC. Destroying the resource disassociates and releases the IP.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Floating IP ID (vttFloatingId). When set, the resource associates this existing Floating IP instead of allocating a new one.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"public_ip": schema.StringAttribute{
				Computed:    true,
				Description: "Public IPv4 address allocated by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the VM instance to associate with. Changing this value forces a new resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_interface_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the NIC to associate with. Changing this value forces a new resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Falls back to the provider default vpc_id when unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *FloatingIPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *FloatingIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FloatingIPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Step 1: Obtain a Floating IP ID — either use an existing one from config or allocate new.
	var floatingID string
	if !plan.ID.IsNull() && plan.ID.ValueString() != "" {
		// Use the existing FIP provided in config — skip allocation.
		floatingID = plan.ID.ValueString()
	} else {
		// Snapshot existing FIP IDs before allocation so we can find the new one
		// in case the allocate response doesn't include an ID.
		beforeIDs, _ := r.listFIPIDs(ctx, vpcID)

		allocBody := map[string]interface{}{
			"number_of_floating_ip": "1",
			"vpc_id":                vpcID,
			"customer_id":           r.customerID,
		}
		allocResp, diags := callAPI(ctx, r.client, pathFloatingIPCreate, allocBody)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		var err error
		floatingID, err = extractFloatingIPID(allocResp)
		if err != nil {
			// Allocate response did not include an ID (e.g. {"status":true}).
			// Find the new FIP by diffing list before vs after.
			floatingID, err = r.findNewFIPID(ctx, vpcID, beforeIDs, 30*time.Second)
			if err != nil {
				resp.Diagnostics.AddError("Cannot determine new Floating IP ID", err.Error())
				return
			}
		}
	}

	plan.ID = types.StringValue(floatingID)
	plan.VpcID = types.StringValue(vpcID)

	// Wait until the FIP has been assigned a public IP (AVAILABLE) before associating.
	// The backend assigns the IP asynchronously after allocation.
	allocDeadline := time.Now().Add(2 * time.Minute)
	for {
		var pollDiags diag.Diagnostics
		r.readInto(ctx, &plan, &pollDiags)
		if pollDiags.HasError() {
			resp.Diagnostics.Append(pollDiags...)
			return
		}
		if plan.PublicIP.ValueString() != "" {
			break
		}
		if time.Now().After(allocDeadline) {
			resp.Diagnostics.AddError(
				"Floating IP allocation timeout",
				fmt.Sprintf("Floating IP %s did not receive a public IP within 2 minutes after allocation", floatingID),
			)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}

	// Step 2: Associate the FIP with the VM instance and NIC (optional).
	if !plan.InstanceID.IsNull() && plan.InstanceID.ValueString() != "" {
		assocBody := map[string]interface{}{
			"floating_ip_id":       floatingID,
			"instance_id":          plan.InstanceID.ValueString(),
			"network_interface_id": plan.NetworkInterfaceID.ValueString(),
			"vpc_id":               vpcID,
			"customer_id":          r.customerID,
		}
		if _, assocDiags := callAPI(ctx, r.client, pathFloatingIPAssociate, assocBody); assocDiags.HasError() {
			// Association failed — attempt cleanup by disassociating/releasing the allocated FIP.
			cleanupBody := map[string]interface{}{
				"floating_ip_id": floatingID,
				"vpc_id":         vpcID,
				"customer_id":    r.customerID,
			}
			_, _ = callAPI(ctx, r.client, pathFloatingIPDisassociate, cleanupBody) // best-effort
			resp.Diagnostics.Append(assocDiags...)
			return
		}
	}

	// Re-read to pick up instance_id / network_interface_id after association.
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FloatingIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FloatingIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.readInto(ctx, &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is intentionally omitted — all attributes that can differ force replacement.

// Update is not supported — all attributes that can differ force replacement.
// This method is required by the resource.Resource interface but should never
// be called because all schema attributes either are Computed or have
// RequiresReplace plan modifiers.
func (r *FloatingIPResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *FloatingIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FloatingIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only disassociate if the FIP was associated with a VM instance.
	if !state.InstanceID.IsNull() && state.InstanceID.ValueString() != "" {
		body := map[string]interface{}{
			"floating_ip_id": state.ID.ValueString(),
			"vpc_id":         state.VpcID.ValueString(),
			"customer_id":    r.customerID,
		}
		apiResp, diags := callAPI(ctx, r.client, pathFloatingIPDisassociate, body)
		if diags.HasError() {
			if apiResp != nil && isNotFoundMessage(apiResp.Message) {
				return // already gone — idempotent
			}
			resp.Diagnostics.Append(diags...)
		}
	}
}

func (r *FloatingIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto fetches /floating-ip/detail and populates m. Returns false ONLY when
// the FIP is gone (drift).
func (r *FloatingIPResource) readInto(ctx context.Context, m *FloatingIPResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"id":         m.ID.ValueString(), // Use "id" so callAPI converts it to int; the API Gateway does not rename "id"
		"vpc_id":     vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathFloatingIPDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	if err := mapFloatingIPResponse(apiResp, m); err != nil {
		diags.AddError("Floating IP response decode failed", err.Error())
	}
	return true
}

// ---------- Pure helpers ----------

// extractFloatingIPID pulls the vttFloatingId out of an allocate or detail response.
func extractFloatingIPID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	// Try string "vttFloatingId" first (detail response).
	if id := asString(data, "vttFloatingId"); id != "" {
		return id, nil
	}
	// Try numeric/string "id" (create response).
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("neither vttFloatingId nor id found in response: %s", string(resp.Data))
}

// mapFloatingIPResponse populates the model from a API detail response.
func mapFloatingIPResponse(resp *client.APIResponse, m *FloatingIPResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}

	if id := asString(data, "vttFloatingId"); id != "" {
		m.ID = types.StringValue(id)
	}
	// floatingIp is the primary field; publicIp/publicIP are alternative names seen in some responses.
	if ip := asString(data, "floatingIp"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	} else if ip := asString(data, "publicIp"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	} else if ip := asString(data, "publicIP"); ip != "" {
		m.PublicIP = types.StringValue(ip)
	}
	if vmID := asIDString(data, "vttVmId"); vmID != "" {
		m.InstanceID = types.StringValue(vmID)
	}
	if nicID := asString(data, "vttNetworkInterfaceId"); nicID != "" {
		m.NetworkInterfaceID = types.StringValue(nicID)
	}
	if vpcID := asIDString(data, "vpcId"); vpcID != "" {
		m.VpcID = types.StringValue(vpcID)
	}
	return nil
}

// listFIPIDs returns the set of FIP IDs currently allocated in the VPC.
// The list API returns {"items": [...], "totalItems": N} where each item has an "id" field (numeric).
// Errors are silently ignored (returns empty set) so callers treat it as best-effort.
func (r *FloatingIPResource) listFIPIDs(ctx context.Context, vpcID string) (map[string]struct{}, error) {
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
		"pageIndex":   0,
		"pageSize":    500,
		"filters":     []interface{}{},
	}
	resp, diags := callAPI(ctx, r.client, pathFloatingIPList, body)
	if diags.HasError() {
		tflog.Warn(ctx, "listFIPIDs: callAPI returned error", map[string]interface{}{"diags": fmt.Sprintf("%v", diags)})
	}
	if resp == nil || len(resp.Data) == 0 {
		tflog.Warn(ctx, "listFIPIDs: empty response")
		return map[string]struct{}{}, nil
	}
	tflog.Debug(ctx, "listFIPIDs: raw response", map[string]interface{}{"data": string(resp.Data)})

	ids := map[string]struct{}{}

	// Response format: {"items": [...], "totalItems": N, "pageIndex": 0, ...}
	// Each item has "id" (numeric) as the FIP identifier.
	var wrapper map[string]json.RawMessage
	if json.Unmarshal(resp.Data, &wrapper) == nil {
		if rawItems, ok := wrapper["items"]; ok {
			var items []map[string]interface{}
			if json.Unmarshal(rawItems, &items) == nil {
				for _, item := range items {
					if id := asIDString(item, "id"); id != "" {
						ids[id] = struct{}{}
					}
				}
			}
		}
	}
	tflog.Debug(ctx, "listFIPIDs: found", map[string]interface{}{"count": len(ids)})
	return ids, nil
}

// findNewFIPID polls the FIP list until a new ID appears that is not in beforeIDs.
func (r *FloatingIPResource) findNewFIPID(ctx context.Context, vpcID string, beforeIDs map[string]struct{}, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		afterIDs, _ := r.listFIPIDs(ctx, vpcID)
		for id := range afterIDs {
			if _, existed := beforeIDs[id]; !existed {
				return id, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("new Floating IP did not appear in list within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
