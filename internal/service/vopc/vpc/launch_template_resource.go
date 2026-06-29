// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = (*LaunchTemplateResource)(nil)
	_ resource.ResourceWithConfigure   = (*LaunchTemplateResource)(nil)
	_ resource.ResourceWithImportState = (*LaunchTemplateResource)(nil)
)

// LaunchTemplateResource implements `viettelidc_launch_template`.
type LaunchTemplateResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// LaunchTemplateResourceModel mirrors the resource schema for State/Plan/Config.
type LaunchTemplateResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VmID        types.String `tfsdk:"vm_id"`
	MemorySize  types.Int64  `tfsdk:"memory_size"`
	CpuSize     types.Int64  `tfsdk:"cpu_size"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

// NewLaunchTemplateResource constructs the resource (registered in iac/provider.go).
func NewLaunchTemplateResource() resource.Resource { return &LaunchTemplateResource{} }

func (r *LaunchTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_launch_template"
}

func (r *LaunchTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Launch Template for Autoscale Groups. " +
			"All attributes are immutable (RequiresReplace / ForceNew) — " +
			"any change destroys the template and creates a new one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Launch Template ID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Launch Template name. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional description. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vm_id": schema.StringAttribute{
				Required:    true,
				Description: "Source VM ID used to seed the template. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"memory_size": schema.Int64Attribute{
				Required:    true,
				Description: "Memory size in GB. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"cpu_size": schema.Int64Attribute{
				Required:    true,
				Description: "Number of vCPUs. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
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

func (r *LaunchTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *LaunchTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LaunchTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := buildLaunchTemplateCreateBody(plan, r.customerID, vpcID)
	apiResp, diags := callAPI(ctx, r.client, pathLaunchTemplateCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := extractLaunchTemplateID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Create response missing launch template ID", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)
	if plan.Description.IsNull() || plan.Description.IsUnknown() {
		plan.Description = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LaunchTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LaunchTemplateResourceModel
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

// Update is not implemented — all attributes are RequiresReplace (ForceNew).
func (r *LaunchTemplateResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *LaunchTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LaunchTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, vpcDiags := resolveVpcID(state.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(vpcDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathLaunchTemplateDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return // Already deleted — idempotent.
		}
		if apiResp != nil && isInUseMessage(apiResp.Message) {
			resp.Diagnostics.AddError(
				"Launch Template In Use",
				"Cannot delete Launch Template — destroy all dependent autoscale_group resources first",
			)
			return
		}
		resp.Diagnostics.Append(diags...)
	}
}

func (r *LaunchTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto fetches launch-template/detail and populates m.
// Returns false ONLY when the template is gone (drift); other errors append to diags.
func (r *LaunchTemplateResource) readInto(ctx context.Context, m *LaunchTemplateResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"id":          m.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathLaunchTemplateDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	if err := mapLaunchTemplateResponse(apiResp, m); err != nil {
		diags.AddError("Launch Template decode error", err.Error())
		return true
	}
	// Fallback: if CSA omits vpcId (e.g. during import), preserve the resolved vpcID.
	if m.VpcID.IsNull() || m.VpcID.IsUnknown() || m.VpcID.ValueString() == "" {
		m.VpcID = types.StringValue(vpcID)
	}
	return true
}

// ---------- Pure helpers (unit-tested) ----------

// buildLaunchTemplateCreateBody constructs the POST body.
func buildLaunchTemplateCreateBody(plan LaunchTemplateResourceModel, customerID, vpcID string) map[string]interface{} {
	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"vm_id":       plan.VmID.ValueString(),
		"memory_size": plan.MemorySize.ValueInt64(),
		"cpu_size":    plan.CpuSize.ValueInt64(),
		"vpc_id":      vpcID,
		"customer_id": customerID,
	}
	// Include description only when explicitly set — allows intentional empty string ("").
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body["description"] = plan.Description.ValueString()
	}
	return body
}

// extractLaunchTemplateID pulls the template ID out of a API create response.
func extractLaunchTemplateID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("launch template ID not present in CSA data: %s", string(resp.Data))
}

// mapLaunchTemplateResponse decodes a CSA launch-template detail payload into the model.
// Returns an error if the response data cannot be decoded — callers must surface this to diagnostics.
func mapLaunchTemplateResponse(resp *client.APIResponse, m *LaunchTemplateResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode launch template detail: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v, ok := data["description"]; ok {
		if s, ok := v.(string); ok {
			m.Description = types.StringValue(s)
		}
	}
	if m.Description.IsNull() || m.Description.IsUnknown() {
		m.Description = types.StringValue("")
	}
	if v := asString(data, "vmId"); v != "" {
		m.VmID = types.StringValue(v)
	}
	// Always set int fields (0 is a valid API value).
	m.MemorySize = types.Int64Value(asInt64(data, "memorySize"))
	m.CpuSize = types.Int64Value(asInt64(data, "cpuSize"))
	if v := asString(data, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	}
	return nil
}
