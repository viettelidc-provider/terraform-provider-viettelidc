// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = (*SubnetResource)(nil)
	_ resource.ResourceWithConfigure   = (*SubnetResource)(nil)
	_ resource.ResourceWithImportState = (*SubnetResource)(nil)
)

// SubnetResource implements `viettelidc_subnet`.
type SubnetResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// SubnetResourceModel mirrors the resource schema for State/Plan/Config marshalling.
type SubnetResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	NetworkAddress types.String `tfsdk:"network_address"`
	IsPublicZone   types.Bool   `tfsdk:"is_public_zone"`
	VpcID          types.String `tfsdk:"vpc_id"`
	Description    types.String `tfsdk:"description"`
}

// NewSubnetResource constructs the resource (registered in iac/provider.go).
func NewSubnetResource() resource.Resource { return &SubnetResource{} }

func (r *SubnetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_subnet"
}

func (r *SubnetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC subnet inside a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Subnet ID assigned by the system (vttSubnetId).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable subnet name.",
			},
			"network_address": schema.StringAttribute{
				Required:    true,
				Description: "CIDR network address (e.g. 10.0.0.0/24). Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"is_public_zone": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether the subnet sits in the public zone. Immutable.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
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
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional description.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SubnetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *SubnetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SubnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := buildSubnetCreateBody(plan, r.customerID, vpcID)
	apiResp, diags := callAPI(ctx, r.client, pathSubnetCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, derr := extractSubnetID(apiResp)
	if derr != nil {
		resp.Diagnostics.AddError("Create response missing vttSubnetId", derr.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)

	// Subnet creation is async; poll until status=AVAILABLE before continuing.
	pollBody := map[string]interface{}{
		"subnet_id":   id,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathSubnetDetail, pollBody, "status", []string{"AVAILABLE", "SUCCESS"}, 10*time.Minute); err != nil {
		resp.Diagnostics.AddError("Subnet did not become ready (AVAILABLE)", err.Error())
		return
	}

	// Sync from CSA to populate computed fields (description, etc.).
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubnetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SubnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.readInto(ctx, &state, &resp.Diagnostics) {
		// Drift: subnet no longer exists.
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SubnetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SubnetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		var diags diag.Diagnostics
		vpcID, diags = resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	body := map[string]interface{}{
		"subnet_id":   state.ID.ValueString(),
		"name":        plan.Name.ValueString(),
		"description": plan.Description.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathSubnetUpdate, body); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	plan.ID = state.ID
	plan.VpcID = types.StringValue(vpcID)
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubnetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SubnetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]interface{}{
		"subnet_id":   state.ID.ValueString(),
		"vpc_id":      state.VpcID.ValueString(),
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathSubnetDelete, body)
	if diags.HasError() {
		// Idempotent: treat already-gone as success.
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until subnet is fully gone from the API (delete is async).
	pollBody := map[string]interface{}{
		"subnet_id":   state.ID.ValueString(),
		"vpc_id":      state.VpcID.ValueString(),
		"customer_id": r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathSubnetDetail, pollBody, 5*time.Minute); err != nil {
		resp.Diagnostics.AddError("Subnet did not disappear after delete", err.Error())
	}
}

func (r *SubnetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto fetches /subnet/detail and populates m. Returns false ONLY when the
// subnet is gone (drift); other errors are appended to diags.
func (r *SubnetResource) readInto(ctx context.Context, m *SubnetResourceModel, diags *diag.Diagnostics) bool {
	body := map[string]interface{}{
		"subnet_id":   m.ID.ValueString(),
		"vpc_id":      m.VpcID.ValueString(),
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathSubnetDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	// Fail fast if subnet entered a terminal error state — prevents dependent
	// resources (VM, NIC, etc.) from being created on a broken subnet.
	if apiResp != nil {
		var raw map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
			if st := asString(raw, "status"); st == "error" || st == "failed" {
				diags.AddError(
					"Subnet is in error state",
					fmt.Sprintf("Subnet %s has status=%s. Destroy and re-create it before proceeding.", m.ID.ValueString(), st),
				)
				return true
			}
		}
	}
	if err := mapSubnetResponse(apiResp, m); err != nil {
		diags.AddError("Subnet response decode failed", err.Error())
	}
	return true
}

// ---------- Pure helpers (unit-tested) ----------

// buildSubnetCreateBody constructs the snake_case POST body.
func buildSubnetCreateBody(plan SubnetResourceModel, customerID, vpcID string) map[string]interface{} {
	body := map[string]interface{}{
		"name":            plan.Name.ValueString(),
		"network_address": plan.NetworkAddress.ValueString(),
		"is_public_zone":  plan.IsPublicZone.ValueBool(),
		"vpc_id":          vpcID,
		"customer_id":     customerID,
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}
	return body
}

// extractSubnetID pulls the subnet ID out of a API create/detail response.
//
// The real API create endpoint returns {"id":"9452","taskId":"...","status":true}.
// The detail endpoint returns {"id":"9452","vttSubnetId":9452,...}.
// We accept the string "id" first, then fall back to numeric "vttSubnetId".
func extractSubnetID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	// Create response: "id" is a string.
	if id, ok := data["id"].(string); ok && id != "" {
		return id, nil
	}
	// Detail response: "vttSubnetId" is a JSON number (float64 in Go).
	if v, ok := data["vttSubnetId"]; ok {
		switch id := v.(type) {
		case float64:
			if id > 0 {
				return fmt.Sprintf("%d", int(id)), nil
			}
		case string:
			if id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("subnet ID not present in CSA data: %s", string(resp.Data))
}

// mapSubnetResponse decodes a CSA subnet detail payload into the model.
// Fields not present in the response are left untouched on the model.
//
// Real API detail shape: {"id":"9452","vttSubnetId":9452,"isPublic":false,...}
// Note: real API uses "isPublic" (not "isPublicZone") and "id" (string).
func mapSubnetResponse(resp *client.APIResponse, m *SubnetResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	// Prefer "id" (string); fall back to "vttSubnetId" (float64 or string).
	if v := asString(data, "id"); v != "" {
		m.ID = types.StringValue(v)
	} else if v, ok := data["vttSubnetId"]; ok {
		switch id := v.(type) {
		case float64:
			if id > 0 {
				m.ID = types.StringValue(fmt.Sprintf("%d", int(id)))
			}
		case string:
			if id != "" {
				m.ID = types.StringValue(id)
			}
		}
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v := asString(data, "networkAddress"); v != "" {
		m.NetworkAddress = types.StringValue(v)
	}
	// Real API uses "isPublic"; fake-api / spec used "isPublicZone".
	// NOTE: The API always returns isPublic=false regardless of the value set on
	// create. To avoid a perpetual destroy/recreate loop, we only populate
	// IsPublicZone from the API when the field is currently null or unknown
	// (e.g. during terraform import). Otherwise the existing plan/state value is
	// preserved so Terraform sees no diff.
	if m.IsPublicZone.IsNull() || m.IsPublicZone.IsUnknown() {
		if _, ok := data["isPublic"]; ok {
			m.IsPublicZone = types.BoolValue(asBool(data, "isPublic"))
		} else if _, ok := data["isPublicZone"]; ok {
			m.IsPublicZone = types.BoolValue(asBool(data, "isPublicZone"))
		} else {
			m.IsPublicZone = types.BoolValue(false)
		}
	}
	if v := asString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	}
	// Description may legitimately be empty; honour explicit presence.
	if v, ok := data["description"]; ok {
		if s, ok := v.(string); ok {
			m.Description = types.StringValue(s)
		}
	}
	if m.Description.IsNull() || m.Description.IsUnknown() {
		m.Description = types.StringValue("")
	}
	return nil
}
