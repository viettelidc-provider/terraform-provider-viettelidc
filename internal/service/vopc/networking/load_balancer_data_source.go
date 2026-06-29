// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource = (*LoadBalancerDataSource)(nil)
)

type LoadBalancerDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type LoadBalancerDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	SubnetID         types.String `tfsdk:"subnet_id"`
	FloatingIPID     types.String `tfsdk:"floating_ip_id"`
	LoadBalancerType types.String `tfsdk:"loadbalancer_type"`
	PackageType      types.String `tfsdk:"package_type"`
	VpcID            types.String `tfsdk:"vpc_id"`
	AdminStateUp     types.Bool   `tfsdk:"admin_state_up"`
	Status           types.String `tfsdk:"status"`
	OperatingStatus  types.String `tfsdk:"operating_status"`
	Listeners        types.List   `tfsdk:"listeners"`
	Pools            types.List   `tfsdk:"pools"`
}

func NewLoadBalancerDataSource() datasource.DataSource { return &LoadBalancerDataSource{} }

func (d *LoadBalancerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_load_balancer"
}

func (d *LoadBalancerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	listenerAttrTypes := map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}

	poolAttrTypes := map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}

	resp.Schema = schema.Schema{
		Description: "Lookup a Load Balancer by ID or name in a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Load Balancer ID (vttLoadBalancerId). Either 'id' or 'name' must be specified.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the Load Balancer to look up. Either 'id' or 'name' must be specified.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to search within. Uses provider default if not specified.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Description of the Load Balancer.",
			},
			"subnet_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the subnet where the Load Balancer is placed.",
			},
			"floating_ip_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the floating IP assigned to the Load Balancer.",
			},
			"loadbalancer_type": schema.StringAttribute{
				Computed:    true,
				Description: "Type of the Load Balancer.",
			},
			"package_type": schema.StringAttribute{
				Computed:    true,
				Description: "Package type of the Load Balancer.",
			},
			"admin_state_up": schema.BoolAttribute{
				Computed:    true,
				Description: "Administrative state of the Load Balancer.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the Load Balancer.",
			},
			"operating_status": schema.StringAttribute{
				Computed:    true,
				Description: "Operating status of the Load Balancer.",
			},
			"listeners": schema.ListAttribute{
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: listenerAttrTypes},
				Description: "List of listeners associated with the Load Balancer.",
			},
			"pools": schema.ListAttribute{
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: poolAttrTypes},
				Description: "List of pools associated with the Load Balancer.",
			},
		},
	}
}

func (d *LoadBalancerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LoadBalancerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config LoadBalancerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(config.VpcID, d.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddError("Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}

	if config.ID.IsNull() && config.Name.IsNull() {
		resp.Diagnostics.AddError("Missing filter", "Either 'id' or 'name' must be specified.")
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
		"pageIndex":   0,
		"pageSize":    1000,
		"filters":     []interface{}{},
	}

	apiResp, diags := callAPI(ctx, d.client, pathLoadBalancerList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	type lbListItem struct {
		VttLoadBalancerID       int64  `json:"vttLoadBalancerId"`
		Name                    string `json:"name"`
		Description             string `json:"description"`
		VttSubnetID             int64  `json:"vttSubnetId"`
		VttLoadbalancerTypeName string `json:"vttLoadbalancerTypeName"`
		LoadbalancerTypeName    string `json:"loadbalancerTypeName"`
		AdminStateUp            bool   `json:"adminStateUp"`
		Status                  string `json:"status"`
		OperatingStatus         string `json:"operatingStatus"`
	}
	var listResp struct {
		Items []lbListItem `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	var found *lbListItem
	for i := range listResp.Items {
		item := &listResp.Items[i]
		if !config.ID.IsNull() && fmt.Sprintf("%d", item.VttLoadBalancerID) == config.ID.ValueString() {
			found = item
			break
		}
		if !config.Name.IsNull() && item.Name == config.Name.ValueString() {
			found = item
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Load Balancer not found with id=%s name=%s", config.ID.ValueString(), config.Name.ValueString()))
		return
	}

	result := LoadBalancerDataSourceModel{
		ID:               types.StringValue(fmt.Sprintf("%d", found.VttLoadBalancerID)),
		Name:             types.StringValue(found.Name),
		Description:      types.StringValue(found.Description),
		VpcID:            types.StringValue(vpcID),
		SubnetID:         types.StringValue(fmt.Sprintf("%d", found.VttSubnetID)),
		FloatingIPID:     types.StringValue(""),
		LoadBalancerType: types.StringValue(found.VttLoadbalancerTypeName),
		PackageType:      types.StringValue(found.LoadbalancerTypeName),
		AdminStateUp:     types.BoolValue(found.AdminStateUp),
		Status:           types.StringValue(found.Status),
		OperatingStatus:  types.StringValue(found.OperatingStatus),
	}

	// Fetch listeners and pools using helper methods
	d.fetchListeners(ctx, &result, &resp.Diagnostics)
	d.fetchPools(ctx, &result, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

func (d *LoadBalancerDataSource) fetchListeners(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerListeners, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		Protocol        string `json:"protocol"`
		ProtocolPort    int    `json:"protocolPort"`
		XForwardedFor   bool   `json:"xForwardedFor"`
		XForwardedPort  bool   `json:"xForwardedPort"`
		XForwardedProto bool   `json:"xForwardedProto"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var listeners []attr.Value
	for _, item := range listResp {
		listenerMap := map[string]attr.Value{
			"id":                types.StringValue(item.ID),
			"name":              types.StringValue(item.Name),
			"description":       types.StringValue(item.Description),
			"protocol":          types.StringValue(item.Protocol),
			"protocol_port":     types.Int64Value(int64(item.ProtocolPort)),
			"x_forwarded_for":   types.BoolValue(item.XForwardedFor),
			"x_forwarded_port":  types.BoolValue(item.XForwardedPort),
			"x_forwarded_proto": types.BoolValue(item.XForwardedProto),
		}
		obj, objDiags := types.ObjectValue(map[string]attr.Type{
			"id":                types.StringType,
			"name":              types.StringType,
			"description":       types.StringType,
			"protocol":          types.StringType,
			"protocol_port":     types.Int64Type,
			"x_forwarded_for":   types.BoolType,
			"x_forwarded_port":  types.BoolType,
			"x_forwarded_proto": types.BoolType,
		}, listenerMap)
		diags.Append(objDiags...)
		listeners = append(listeners, obj)
	}

	listType, listDiags := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}}, listeners)
	diags.Append(listDiags...)
	model.Listeners = listType
}

func (d *LoadBalancerDataSource) fetchPools(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerPools, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID                     string `json:"id"`
		Name                   string `json:"name"`
		Description            string `json:"description"`
		Algorithm              string `json:"algorithm"`
		SessionPersistenceType string `json:"sessionPersistenceType"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var pools []attr.Value
	for _, item := range listResp {
		poolMap := map[string]attr.Value{
			"id":                       types.StringValue(item.ID),
			"name":                     types.StringValue(item.Name),
			"description":              types.StringValue(item.Description),
			"algorithm":                types.StringValue(item.Algorithm),
			"session_persistence_type": types.StringValue(item.SessionPersistenceType),
		}
		obj, objDiags := types.ObjectValue(map[string]attr.Type{
			"id":                       types.StringType,
			"name":                     types.StringType,
			"description":              types.StringType,
			"algorithm":                types.StringType,
			"session_persistence_type": types.StringType,
		}, poolMap)
		diags.Append(objDiags...)
		pools = append(pools, obj)
	}

	listType, listDiags := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}}, pools)
	diags.Append(listDiags...)
	model.Pools = listType
}
