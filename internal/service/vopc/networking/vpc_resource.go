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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = (*VPCResource)(nil)
	_ resource.ResourceWithConfigure   = (*VPCResource)(nil)
	_ resource.ResourceWithImportState = (*VPCResource)(nil)
)

// VPCResource implements `viettelidc_vpc`.
type VPCResource struct {
	client     *client.Client
	customerID string
}

// VPCResourceModel mirrors the resource schema.
type VPCResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	CidrBlock   types.String `tfsdk:"cidr_block"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
}

// NewVPCResource constructs the resource (registered in iac/provider.go).
func NewVPCResource() resource.Resource { return &VPCResource{} }

func (r *VPCResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_vpc"
}

func (r *VPCResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Quản lý Virtual Private Cloud (VPC) trên ViettelIDC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "ID của VPC (do hệ thống cấp).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Tên VPC.",
			},
			"cidr_block": schema.StringAttribute{
				Required:    true,
				Description: "Dải địa chỉ IP của VPC theo định dạng CIDR (ví dụ: 10.0.0.0/16). Không thể thay đổi sau khi tạo.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Mô tả VPC.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Trạng thái VPC (success, pending, error...).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *VPCResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
}

func (r *VPCResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VPCResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"cidr_block":  plan.CidrBlock.ValueString(),
		"customer_id": r.customerID,
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}

	apiResp, diags := callAPI(ctx, r.client, pathVPCCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := extractVPCID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Không lấy được ID VPC sau khi tạo", err.Error())
		return
	}

	plan.ID = types.StringValue(id)

	// Poll until the VPC reaches a ready state before reading full details.
	pollBody := map[string]interface{}{
		"vpc_id":      id,
		"customer_id": r.customerID,
	}
	if err := pollUntilReady(ctx, r.client, pathVPCDetail, pollBody, 2*time.Minute); err != nil {
		// Save partial state with the ID so the resource is not orphaned.
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("VPC did not become ready", err.Error())
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VPCResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VPCResourceModel
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

func (r *VPCResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state VPCResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      state.ID.ValueString(),
		"name":        plan.Name.ValueString(),
		"description": plan.Description.ValueString(),
		"customer_id": r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathVPCUpdate, body); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	plan.ID = state.ID
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VPCResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VPCResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      state.ID.ValueString(),
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathVPCDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until VPC is fully gone from the API (delete is async).
	pollBody := map[string]interface{}{
		"vpc_id":      state.ID.ValueString(),
		"customer_id": r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathVPCDetail, pollBody, 5*time.Minute); err != nil {
		resp.Diagnostics.AddError("VPC did not disappear after delete", err.Error())
	}
}

func (r *VPCResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto fetches VPC detail and populates m. Returns false when VPC không tồn tại (drift).
func (r *VPCResource) readInto(ctx context.Context, m *VPCResourceModel, diags *diag.Diagnostics) bool {
	body := map[string]interface{}{
		"vpc_id":      m.ID.ValueString(),
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathVPCDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	// Fail fast if VPC entered a terminal error state.
	if apiResp != nil {
		var raw map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &raw); err == nil {
			if st := asString(raw, "status"); st == "error" || st == "failed" || st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"VPC is in error state",
					fmt.Sprintf("VPC %s has status=%s. Destroy and re-create it before proceeding.", m.ID.ValueString(), st),
				)
				return true
			}
		}
	}
	if err := mapVPCResponse(apiResp, m); err != nil {
		diags.AddError("Không giải mã được thông tin VPC", err.Error())
	}
	return true
}

// ---------- Helpers ----------

func extractVPCID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("giải mã data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	if id := asIDString(data, "vttVpcId"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("ID VPC không có trong phản hồi: %s", string(resp.Data))
}

func mapVPCResponse(resp *client.APIResponse, m *VPCResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("giải mã data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	} else if id := asIDString(data, "vttVpcId"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v := asString(data, "cidrBlock"); v != "" {
		m.CidrBlock = types.StringValue(v)
	} else if v := asString(data, "cidr_block"); v != "" {
		m.CidrBlock = types.StringValue(v)
	}
	if v := asString(data, "description"); v != "" {
		m.Description = types.StringValue(v)
	} else if m.Description.IsUnknown() {
		m.Description = types.StringValue("")
	}
	if v := asString(data, "status"); v != "" {
		m.Status = types.StringValue(v)
	} else if m.Status.IsUnknown() {
		m.Status = types.StringValue("")
	}
	return nil
}
