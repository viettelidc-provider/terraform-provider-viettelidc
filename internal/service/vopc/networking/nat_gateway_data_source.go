// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource = (*NatGatewayDataSource)(nil)
)

type NatGatewayDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type NatGatewayDataSourceModel struct {
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

func NewNatGatewayDataSource() datasource.DataSource { return &NatGatewayDataSource{} }

func (d *NatGatewayDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_nat_gateway"
}

func (d *NatGatewayDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lookup a NAT Gateway by ID or name in a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "NAT Gateway ID (vttNatId). Either 'id' or 'name' must be specified.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the NAT Gateway to look up. Either 'id' or 'name' must be specified.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to search within. Uses provider default if not specified.",
			},
			"subnet_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the subnet where the NAT Gateway is placed.",
			},
			"internet_gateway_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the Internet Gateway used for outbound traffic.",
			},
			"connect_type": schema.BoolAttribute{
				Computed:    true,
				Description: "Connection type. True if using dedicated connection.",
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
				Description: "Current status of the NAT Gateway.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Timestamp when the NAT Gateway was created.",
			},
		},
	}
}

func (d *NatGatewayDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NatGatewayDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NatGatewayDataSourceModel
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
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}

	apiResp, diags := callAPI(ctx, d.client, pathNatGatewayList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	type natItem struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		VttSubnetID int64  `json:"vttSubnetId"`
		ConnectType bool   `json:"connectType"`
		NicIP       string `json:"nicIp"`
		Status      string `json:"status"`
		CreatedAt   string `json:"createdAt"`
	}
	var listResp struct {
		Items []natItem `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	var found *natItem
	for i, item := range listResp.Items {
		if !config.ID.IsNull() && fmt.Sprintf("%d", item.ID) == config.ID.ValueString() {
			found = &listResp.Items[i]
			break
		}
		if !config.Name.IsNull() && item.Name == config.Name.ValueString() {
			found = &listResp.Items[i]
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("NAT Gateway not found with id=%s name=%s", config.ID.ValueString(), config.Name.ValueString()))
		return
	}

	result := NatGatewayDataSourceModel{
		ID:                types.StringValue(fmt.Sprintf("%d", found.ID)),
		Name:              types.StringValue(found.Name),
		SubnetID:          types.StringValue(fmt.Sprintf("%d", found.VttSubnetID)),
		InternetGatewayID: types.StringValue(""),
		ConnectType:       types.BoolValue(found.ConnectType),
		VpcID:             types.StringValue(vpcID),
		FloatingIPID:      types.StringValue(""),
		FloatingIP:        types.StringValue(found.NicIP),
		Status:            types.StringValue(found.Status),
		CreatedAt:         types.StringValue(found.CreatedAt),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}
