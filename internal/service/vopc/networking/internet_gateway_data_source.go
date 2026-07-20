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
	_ datasource.DataSource = (*InternetGatewayDataSource)(nil)
)

type InternetGatewayDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type InternetGatewayDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	VpcID      types.String `tfsdk:"vpc_id"`
	Status     types.String `tfsdk:"status"`
	SubnetID   types.String `tfsdk:"subnet_id"`
	FloatingIP types.String `tfsdk:"floating_ip"`
}

func NewInternetGatewayDataSource() datasource.DataSource { return &InternetGatewayDataSource{} }

func (d *InternetGatewayDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_internet_gateway"
}

func (d *InternetGatewayDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lookup an Internet Gateway by ID or name in a VPC. Internet Gateways are platform-managed resources that can only be listed and referenced.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Internet Gateway ID (vttInternetGatewayId). Either 'id' or 'name' must be specified.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the Internet Gateway to look up. Either 'id' or 'name' must be specified.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to search within. Uses provider default if not specified.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the Internet Gateway.",
			},
			"subnet_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the subnet associated with the Internet Gateway.",
			},
			"floating_ip": schema.StringAttribute{
				Computed:    true,
				Description: "The floating IP address assigned to the Internet Gateway.",
			},
		},
	}
}

func (d *InternetGatewayDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *InternetGatewayDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config InternetGatewayDataSourceModel
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
		"vpcId":      vpcID,
		"customerId": d.customerID,
	}

	apiResp, diags := callAPI(ctx, d.client, pathInternetGatewayList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Response shape: {"pageIndex":0,"pageSize":10,"totalItems":N,"items":[{"id":...,"name":...}]}
	var listResp struct {
		Items []struct {
			ID                int64  `json:"id"`
			Name              string `json:"name"`
			Status            string `json:"status"`
			VttSubnetID       int64  `json:"vttSubnetId"`
			FloatingIPAddress string `json:"floatingIpAddress"`
		} `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	var found *struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		Status            string `json:"status"`
		VttSubnetID       int64  `json:"vttSubnetId"`
		FloatingIPAddress string `json:"floatingIpAddress"`
	}

	for _, item := range listResp.Items {
		if !config.ID.IsNull() && fmt.Sprintf("%d", item.ID) == config.ID.ValueString() {
			found = &item
			break
		}
		if !config.Name.IsNull() && item.Name == config.Name.ValueString() {
			found = &item
			break
		}
	}

	if found == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Internet Gateway not found with id=%s name=%s", config.ID.ValueString(), config.Name.ValueString()))
		return
	}

	result := InternetGatewayDataSourceModel{
		ID:         types.StringValue(fmt.Sprintf("%d", found.ID)),
		Name:       types.StringValue(found.Name),
		VpcID:      types.StringValue(vpcID),
		Status:     types.StringValue(found.Status),
		SubnetID:   types.StringValue(fmt.Sprintf("%d", found.VttSubnetID)),
		FloatingIP: types.StringValue(found.FloatingIPAddress),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}
