// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*InternetGatewaysDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*InternetGatewaysDataSource)(nil)
)

// InternetGatewaysDataSource implements `data "viettelidc_ovpc_internet_gateways"`.
// The singular data source needs a name or an ID, but IGW names are generated
// per VPC (e.g. "ig-d9112826"), so without this there is no way to discover one
// from Terraform.
type InternetGatewaysDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type InternetGatewaysDataSourceModel struct {
	VpcID            types.String          `tfsdk:"vpc_id"`
	InternetGateways []InternetGatewayItem `tfsdk:"internet_gateways"`
}

type InternetGatewayItem struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
}

func NewInternetGatewaysDataSource() datasource.DataSource { return &InternetGatewaysDataSource{} }

func (d *InternetGatewaysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_internet_gateways"
}

func (d *InternetGatewaysDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List all Internet Gateways in a VPC. Use this to discover the generated IGW name/ID needed by viettelidc_ovpc_nat_gateway.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC filter; falls back to the provider default vpc_id.",
			},
			"internet_gateways": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":     schema.StringAttribute{Computed: true},
						"name":   schema.StringAttribute{Computed: true},
						"status": schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *InternetGatewaysDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *InternetGatewaysDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg InternetGatewaysDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
		"pageIndex":   0,
		"pageSize":    1000,
	}
	apiResp, diags := callAPI(ctx, d.client, pathInternetGatewayList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := decodeSubnetList(apiResp) // shape-generic list decoder
	if err != nil {
		resp.Diagnostics.AddError("decode internet gateway list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.InternetGateways = make([]InternetGatewayItem, 0, len(items))
	for _, raw := range items {
		cfg.InternetGateways = append(cfg.InternetGateways, InternetGatewayItem{
			ID:     types.StringValue(asIDString(raw, "id")),
			Name:   types.StringValue(asString(raw, "name")),
			Status: types.StringValue(asString(raw, "status")),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
