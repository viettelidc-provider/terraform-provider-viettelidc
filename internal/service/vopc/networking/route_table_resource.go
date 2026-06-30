// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*RouteTableResource)(nil)
	_ resource.ResourceWithConfigure   = (*RouteTableResource)(nil)
	_ resource.ResourceWithImportState = (*RouteTableResource)(nil)
)

// RouteTableResource implements `viettelidc_route_table`.
//
// NOTE: The API does not expose a delete endpoint for route tables.
// Terraform will remove the resource from state on destroy, but the actual
// route table is NOT deleted from the cloud.
type RouteTableResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type RouteTableResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	VpcID types.String `tfsdk:"vpc_id"`
}

func NewRouteTableResource() resource.Resource { return &RouteTableResource{} }

func (r *RouteTableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_route_table"
}

func (r *RouteTableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Route Table. NOTE: The API has no delete endpoint; destroying this resource removes it from Terraform state only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Route Table ID (vttRouteTableId).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Route Table name.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Falls back to provider default when unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *RouteTableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *RouteTableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RouteTableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// CSA route-table/create requires both "vpc" (int) and "vpc_id" (renamed to vpcId by the API Gateway).
	vpcIDInt, _ := strconv.ParseInt(vpcID, 10, 64)
	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"vpc":         vpcIDInt,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}

	apiResp, diags := callAPI(ctx, r.client, pathRouteTableCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rtID, err := extractRouteTableID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Route Table create response missing id", err.Error())
		return
	}

	plan.ID = types.StringValue(rtID)
	plan.VpcID = types.StringValue(vpcID)

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RouteTableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RouteTableResourceModel
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

func (r *RouteTableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No update endpoint: only name is mutable, and there is no update API.
	// Plan modifiers do not include RequiresReplace for name, so Update will
	// be called but we simply re-read to keep state consistent.
	var plan, state RouteTableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Name cannot actually be changed via API — warn the user.
	if plan.Name.ValueString() != state.Name.ValueString() {
		resp.Diagnostics.AddWarning(
			"Route Table name change not supported",
			"The API does not expose an update endpoint for Route Tables. The name in state will remain unchanged.",
		)
	}

	plan.ID = state.ID
	plan.Name = state.Name
	plan.VpcID = state.VpcID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the Route Table via the API.
func (r *RouteTableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RouteTableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"route_table_id": state.ID.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    r.customerID,
	}

	apiResp, diags := callAPI(ctx, r.client, pathRouteTableDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
	}
}

func (r *RouteTableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------- Helpers ----------

func (r *RouteTableResource) readInto(ctx context.Context, m *RouteTableResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"route_table_id": m.ID.ValueString(),
		"vpc_id":         vpcID,
		"customer_id":    r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathRouteTableDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	if err := mapRouteTableResponse(apiResp, m); err != nil {
		diags.AddError("Route Table detail decode failed", err.Error())
	}
	return true
}

func extractRouteTableID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttRouteTableId"); id != "" {
		return id, nil
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("id not found in response: %s", string(resp.Data))
}

func mapRouteTableResponse(resp *client.APIResponse, m *RouteTableResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttRouteTableId"); id != "" {
		m.ID = types.StringValue(id)
	} else if id := asIDString(data, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if vpcID := asIDString(data, "vpcId"); vpcID != "" {
		m.VpcID = types.StringValue(vpcID)
	}
	return nil
}
