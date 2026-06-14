package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

const (
	ipAssignStatic  = "STATIC"
	ipAssignDynamic = "auto"
)

var (
	_ resource.Resource                   = (*NetworkInterfaceResource)(nil)
	_ resource.ResourceWithConfigure      = (*NetworkInterfaceResource)(nil)
	_ resource.ResourceWithImportState    = (*NetworkInterfaceResource)(nil)
	_ resource.ResourceWithValidateConfig = (*NetworkInterfaceResource)(nil)
)

// NetworkInterfaceResource implements `viettelidc_network_interface`.
type NetworkInterfaceResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NetworkInterfaceResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	SubnetID     types.String `tfsdk:"subnet_id"`
	IpAssignType types.String `tfsdk:"ip_assign_type"`
	IpAddress    types.String `tfsdk:"ip_address"`
	VpcID        types.String `tfsdk:"vpc_id"`
	Description  types.String `tfsdk:"description"`
	Status       types.String `tfsdk:"status"`
}

func NewNetworkInterfaceResource() resource.Resource { return &NetworkInterfaceResource{} }

func (r *NetworkInterfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_network_interface"
}

func (r *NetworkInterfaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Network Interface (NIC).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "NIC ID assigned by the system (vttNetworkInterfaceId).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":      schema.StringAttribute{Required: true},
			"subnet_id": schema.StringAttribute{Required: true, Description: "Subnet to attach the NIC to. Mutable (NIC may move subnets)."},
			"ip_assign_type": schema.StringAttribute{
				Required:    true,
				Description: "STATIC or auto. Immutable.",
				Validators: []validator.String{
					stringvalidator.OneOf(ipAssignStatic, ipAssignDynamic),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ip_address": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "IP address. Required when ip_assign_type=STATIC; assigned by the system when auto.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *NetworkInterfaceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg NetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.IpAssignType.ValueString() == ipAssignDynamic &&
		!cfg.IpAddress.IsNull() && !cfg.IpAddress.IsUnknown() && cfg.IpAddress.ValueString() != "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("ip_address"),
			"Invalid Configuration",
					"ip_address must NOT be set when ip_assign_type is auto (CSA will assign one).",
		)
	}
}

func (r *NetworkInterfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *NetworkInterfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := buildNicCreateBody(plan, r.customerID, vpcID)
	apiResp, diags := callAPI(ctx, r.client, pathNicCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := extractNicID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Create response missing vttNetworkInterfaceId", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)

	// NIC creation is async; poll until status=AVAILABLE before reading state.
	pollBody := map[string]interface{}{
		"network_interface_id": id,
		"vpc_id":               vpcID,
		"customer_id":          r.customerID,
	}
	if err := pollForStatus(ctx, r.client, pathNicDetail, pollBody, "status", []string{"AVAILABLE", "SUCCESS"}, 10*time.Minute); err != nil {
		resp.Diagnostics.AddError("NIC did not become ready (AVAILABLE)", err.Error())
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkInterfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkInterfaceResourceModel
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

func (r *NetworkInterfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state NetworkInterfaceResourceModel
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
		"network_interface_id": state.ID.ValueString(),
		"name":                 plan.Name.ValueString(),
		"subnet_id":            plan.SubnetID.ValueString(),
		"vpc_id":               vpcID,
		"customer_id":          r.customerID,
	}
	if plan.IpAddress.ValueString() != "" {
		body["ip_address"] = plan.IpAddress.ValueString()
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}
	if _, diags := callAPI(ctx, r.client, pathNicUpdate, body); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	plan.ID = state.ID
	plan.IpAssignType = state.IpAssignType // immutable
	plan.VpcID = types.StringValue(vpcID)
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkInterfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkInterfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteBody := map[string]interface{}{
		"network_interface_id": state.ID.ValueString(),
		"vpc_id":               state.VpcID.ValueString(),
		"customer_id":          r.customerID,
	}
	// Attempt delete directly. CSA returns HTTP 500 for pending NICs;
	// treat that as "stuck pending" and abandon to let destroy complete.
	apiResp, diags := callAPI(ctx, r.client, pathNicDelete, deleteBody)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		// HTTP 500 likely means NIC is still in pending state and cannot be deleted.
		// Abandon with a warning so destroy can complete without blocking 2+ minutes.
		resp.Diagnostics.AddWarning(
			"NIC could not be deleted, abandoning",
			fmt.Sprintf("NIC %s delete failed: %s. The resource has been removed from Terraform state. The underlying NIC may need manual cleanup.", state.ID.ValueString(), diags[0].Detail()),
		)
		return
	}

	// Poll until NIC is fully gone from the API (delete is async).
	pollBody := map[string]interface{}{
		"network_interface_id": state.ID.ValueString(),
		"vpc_id":               state.VpcID.ValueString(),
		"customer_id":          r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathNicDetail, pollBody, 2*time.Minute); err != nil {
		resp.Diagnostics.AddError("NIC did not disappear after delete", err.Error())
	}
}

func (r *NetworkInterfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *NetworkInterfaceResource) readInto(ctx context.Context, m *NetworkInterfaceResourceModel, diags *diag.Diagnostics) bool {
	body := map[string]interface{}{
		"network_interface_id": m.ID.ValueString(),
		"vpc_id":               m.VpcID.ValueString(),
		"customer_id":          r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathNicDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	// Fail fast if NIC entered a terminal error state.
	if apiResp != nil {
		var raw map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
			if st := asString(raw, "status"); st == "error" || st == "failed" || st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"Network Interface is in error state",
					fmt.Sprintf("NIC %s has status=%s. Destroy and re-create it before proceeding.", m.ID.ValueString(), st),
				)
				return true
			}
		}
	}
	if err := mapNicResponse(apiResp, m); err != nil {
		diags.AddError("NIC response decode failed", err.Error())
	}
	return true
}

// ---------- Pure helpers ----------

func buildNicCreateBody(plan NetworkInterfaceResourceModel, customerID, vpcID string) map[string]interface{} {
	body := map[string]interface{}{
		"name":           plan.Name.ValueString(),
		"subnet_id":      plan.SubnetID.ValueString(),
		"ip_assign_type": plan.IpAssignType.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    customerID,
	}
	if plan.IpAssignType.ValueString() == ipAssignStatic && plan.IpAddress.ValueString() != "" {
		body["ip_address"] = plan.IpAddress.ValueString()
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}
	return body
}

func extractNicID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	// Real API create response: {"id":"<str>", "taskId":"...", "status":true}
	if id, ok := data["id"].(string); ok && id != "" {
		return id, nil
	}
	// Fake-api / legacy format
	id := asIDString(data, "vttNetworkInterfaceId")
	if id == "" {
		return "", fmt.Errorf("NIC ID not present in response: %s", string(resp.Data))
	}
	return id, nil
}

func mapNicResponse(resp *client.APIResponse, m *NetworkInterfaceResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	fillNicFromMap(data, m)
	return nil
}

func fillNicFromMap(data map[string]interface{}, m *NetworkInterfaceResourceModel) {
	// Real API detail: "id" is the NIC ID (string). Fake-api uses "vttNetworkInterfaceId".
	if id, ok := data["id"].(string); ok && id != "" {
		m.ID = types.StringValue(id)
	} else if v := asIDString(data, "vttNetworkInterfaceId"); v != "" {
		m.ID = types.StringValue(v)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	// Real API returns vttSubnetId as an integer; asIDString handles both int and string.
	if v := asIDString(data, "vttSubnetId"); v != "" {
		m.SubnetID = types.StringValue(v)
	}
	if v := asString(data, "ipAssignType"); v != "" {
		m.IpAssignType = types.StringValue(v)
	}
	// ipAddress may be absent when NIC is pending; always set to avoid unknown-value errors.
	if v := asString(data, "ipAddress"); v != "" {
		m.IpAddress = types.StringValue(v)
	} else if m.IpAddress.IsNull() || m.IpAddress.IsUnknown() {
		m.IpAddress = types.StringValue("")
	}
	// vpcId in NIC detail is 0 (not the real VPC); only update if non-zero.
	if v := asIDString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	}
	// description is not returned by NIC detail; preserve plan/state value.
	if v := asString(data, "description"); v != "" {
		m.Description = types.StringValue(v)
	} else if m.Description.IsUnknown() {
		m.Description = types.StringNull()
	}
	m.Status = types.StringValue(asString(data, "status"))
}
