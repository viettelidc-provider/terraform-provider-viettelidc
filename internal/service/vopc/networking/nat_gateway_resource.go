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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*NatGatewayResource)(nil)
	_ resource.ResourceWithConfigure   = (*NatGatewayResource)(nil)
	_ resource.ResourceWithImportState = (*NatGatewayResource)(nil)
)

type NatGatewayResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NatGatewayResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	SubnetID          types.String `tfsdk:"subnet_id"`
	InternetGatewayID types.String `tfsdk:"internet_gateway_id"`
	ConnectType       types.Bool   `tfsdk:"connect_type"`
	VpcID             types.String `tfsdk:"vpc_id"`
	FloatingIP        types.String `tfsdk:"floating_ip"`
	FloatingIPID      types.String `tfsdk:"floating_ip_id"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

func NewNatGatewayResource() resource.Resource { return &NatGatewayResource{} }

func (r *NatGatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_nat_gateway"
}

func (r *NatGatewayResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC NAT Gateway allows instances in a private subnet to connect to the internet.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "NAT Gateway ID assigned by the system (vttNatId).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable NAT Gateway name.",
			},
			"subnet_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the subnet where the NAT Gateway will be placed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"internet_gateway_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the Internet Gateway to use for outbound traffic.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"connect_type": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Connection type. If true, uses dedicated connection.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
			},
			"floating_ip": schema.StringAttribute{
				Computed:    true,
				Description: "The floating IP address assigned to the NAT Gateway.",
			},
			"floating_ip_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the floating IP assigned to the NAT Gateway.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the NAT Gateway (e.g., ACTIVE, PENDING).",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Timestamp when the NAT Gateway was created.",
			},
		},
	}
}

func (r *NatGatewayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *NatGatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NatGatewayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(plan.VpcID, r.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("vpc_id"), "Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}

	body := map[string]interface{}{
		"vpc_id":                  parseInt(vpcID),
		"customer_id":             parseInt(r.customerID),
		"name":                    plan.Name.ValueString(),
		"vtt_subnet_id":           parseInt(plan.SubnetID.ValueString()),
		"vtt_internet_gateway_id": parseInt(plan.InternetGatewayID.ValueString()),
		"connect_type":            plan.ConnectType.ValueBool(),
	}

	apiResp, diags := callAPI(ctx, r.client, pathNatGatewayCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result struct {
		ID     string `json:"id"`
		Status bool   `json:"status"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	plan.ID = types.StringValue(result.ID)
	plan.VpcID = types.StringValue(vpcID)

	// Poll until the NAT Gateway reaches a terminal ready state and report to user.
	if err := r.pollReady(ctx, &plan, 5*time.Minute); err != nil {
		resp.Diagnostics.AddError("NAT Gateway did not become ready", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *NatGatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NatGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *NatGatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// NAT Gateway doesn't support update via API
	resp.Diagnostics.AddError("Update Not Supported", "NAT Gateway does not support in-place updates. Recreate the resource instead.")
}

func (r *NatGatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NatGatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      state.VpcID.ValueString(),
		"customer_id": r.customerID,
		"vtt_nat_id":  state.ID.ValueString(),
	}

	apiResp, diags := callAPI(ctx, r.client, pathNatGatewayDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
	}
}

func (r *NatGatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *NatGatewayResource) readAndMerge(ctx context.Context, model *NatGatewayResourceModel, diags *diag.Diagnostics) {
	// Use defaultVpcID fallback when VpcID is empty (e.g. after import).
	vpcID := model.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
		if vpcID != "" {
			model.VpcID = types.StringValue(vpcID)
		}
	}
	if vpcID == "" || model.ID.ValueString() == "" {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathNatGatewayList, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp struct {
		Items []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			VttSubnetID int64  `json:"vttSubnetId"`
			ConnectType bool   `json:"connectType"`
			NicIP       string `json:"nicIp"`
			Status      string `json:"status"`
			CreatedAt   string `json:"createdAt"`
			VpcID       int64  `json:"vpcId"`
		} `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	targetID := model.ID.ValueString()
	for _, item := range listResp.Items {
		if fmt.Sprintf("%d", item.ID) == targetID {
			model.Name = types.StringValue(item.Name)
			model.SubnetID = types.StringValue(fmt.Sprintf("%d", item.VttSubnetID))
			model.ConnectType = types.BoolValue(item.ConnectType)
			model.FloatingIP = types.StringValue(item.NicIP)
			model.FloatingIPID = types.StringValue("")
			model.Status = types.StringValue(item.Status)
			model.CreatedAt = types.StringValue(item.CreatedAt)
			if item.VpcID != 0 {
				model.VpcID = types.StringValue(fmt.Sprintf("%d", item.VpcID))
			}
			// Fail fast if NAT Gateway entered a terminal error state.
			if st := strings.ToUpper(item.Status); st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"NAT Gateway is in error state",
					fmt.Sprintf("NAT Gateway %s has status=%s. Destroy and re-create it before proceeding.", targetID, item.Status),
				)
			}
			return
		}
	}

	diags.AddError("Not Found", fmt.Sprintf("NAT Gateway %s not found", targetID))
}

// pollReady polls the NAT Gateway list endpoint every 10 seconds until the gateway
// reaches a terminal ready state, an error state, or the timeout elapses.
func (r *NatGatewayResource) pollReady(ctx context.Context, model *NatGatewayResourceModel, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var diags diag.Diagnostics
		r.readAndMerge(ctx, model, &diags)
		if diags.HasError() {
			for _, diagErr := range diags.Errors() {
				summary := strings.ToLower(diagErr.Summary())
				detail := strings.ToLower(diagErr.Detail())
				if strings.Contains(summary, "error state") || strings.Contains(detail, "error state") {
					return fmt.Errorf("NAT Gateway entered error state (status=%s)", model.Status.ValueString())
				}
			}
		} else {
			switch strings.ToUpper(model.Status.ValueString()) {
			case "ACTIVE", "SUCCESS", "AVAILABLE":
				return nil
			case "ERROR", "FAILED":
				return fmt.Errorf("NAT Gateway entered error state (status=%s)", model.Status.ValueString())
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for NAT Gateway to become ready (timeout=%s, last status=%s)", timeout, model.Status.ValueString())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}
