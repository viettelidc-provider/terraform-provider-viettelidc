// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

var (
	_ resource.Resource                = (*SecurityGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*SecurityGroupResource)(nil)
	_ resource.ResourceWithImportState = (*SecurityGroupResource)(nil)
)

// SecurityGroupResource implements `viettelidc_security_group`.
type SecurityGroupResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// SecurityGroupResourceModel mirrors the resource schema for State/Plan/Config marshalling.
type SecurityGroupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

// NewSecurityGroupResource constructs the resource (registered in iac/provider.go).
func NewSecurityGroupResource() resource.Resource { return &SecurityGroupResource{} }

func (r *SecurityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group"
}

func (r *SecurityGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Security Group — a named firewall container that holds inbound/outbound rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Security Group ID assigned by the system (vttSecurityGroupId).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Security Group name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional description.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

func (r *SecurityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *SecurityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"name":        plan.Name.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
		"rules":       []interface{}{},
	}
	if d := plan.Description.ValueString(); d != "" {
		body["description"] = d
	}

	apiResp, diags := callAPI(ctx, r.client, pathSGCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := extractSGID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Create response missing vttSecurityGroupId", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)

	// Wait for the Security Group to become ready before reading state.
	pollBody := map[string]interface{}{
		"security_group_id": id,
		"vpc_id":            vpcID,
		"customer_id":       r.customerID,
	}
	if err := pollUntilReady(ctx, r.client, pathSGDetail, pollBody, 2*time.Minute); err != nil {
		resp.Diagnostics.AddError("Security Group did not become ready", err.Error())
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecurityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupResourceModel
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

func (r *SecurityGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecurityGroupResourceModel
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
		"security_group_id": state.ID.ValueString(),
		"name":              plan.Name.ValueString(),
		"description":       plan.Description.ValueString(),
		"vpc_id":            vpcID,
		"customer_id":       r.customerID,
	}
	if _, diags := callAPI(ctx, r.client, pathSGUpdate, body); diags.HasError() {
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

func (r *SecurityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"security_group_id": state.ID.ValueString(),
		"vpc_id":            state.VpcID.ValueString(),
		"customer_id":       r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathSGDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return // idempotent
		}
		if apiResp != nil && isSGInUseMessage(apiResp.Message) {
			resp.Diagnostics.AddError(
				"Security Group In Use",
				"Cannot delete Security Group while attached to instances. Detach all instances first.",
			)
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until the SG is fully gone from the API (delete is async on the backend).
	pollBody := map[string]interface{}{
		"security_group_id": state.ID.ValueString(),
		"vpc_id":            state.VpcID.ValueString(),
		"customer_id":       r.customerID,
	}
	if err := pollUntilGone(ctx, r.client, pathSGDetail, pollBody, 2*time.Minute); err != nil {
		resp.Diagnostics.AddError("Security Group did not disappear after delete", err.Error())
	}
}

func (r *SecurityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto fetches /security-group/detail and populates m. Returns false when the SG is gone.
func (r *SecurityGroupResource) readInto(ctx context.Context, m *SecurityGroupResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	body := map[string]interface{}{
		"security_group_id": m.ID.ValueString(),
		"vpc_id":            vpcID,
		"customer_id":       r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathSGDetail, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	if err := mapSGResponse(apiResp, m); err != nil {
		diags.AddError("Security Group response decode failed", err.Error())
	}
	return true
}

// ---------- Pure helpers ----------

func extractSGID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttSecurityGroupId"); id != "" {
		return id, nil
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("neither vttSecurityGroupId nor id found in response: %s", string(resp.Data))
}

func mapSGResponse(resp *client.APIResponse, m *SecurityGroupResourceModel) error {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "vttSecurityGroupId"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(data, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	m.Description = types.StringValue(asString(data, "description"))
	if vpcID := asIDString(data, "vpcId"); vpcID != "" {
		m.VpcID = types.StringValue(vpcID)
	}
	return nil
}

func isSGInUseMessage(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "in use") || strings.Contains(m, "attached") || strings.Contains(m, "being used")
}
