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
	_ datasource.DataSource              = (*SubnetsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*SubnetsDataSource)(nil)
)

// SubnetsDataSource implements `data "viettelidc_subnets"` (list/all).
type SubnetsDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type SubnetsDataSourceModel struct {
	VpcID   types.String `tfsdk:"vpc_id"`
	Subnets []SubnetItem `tfsdk:"subnets"`
}

type SubnetItem struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	NetworkAddress types.String `tfsdk:"network_address"`
	IsPublicZone   types.Bool   `tfsdk:"is_public_zone"`
	Description    types.String `tfsdk:"description"`
}

func NewSubnetsDataSource() datasource.DataSource { return &SubnetsDataSource{} }

func (d *SubnetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_subnets"
}

func (d *SubnetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List all ViettelIDC subnets in a VPC.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{Optional: true, Description: "VPC filter; falls back to provider default."},
			"subnets": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{Computed: true},
						"name":            schema.StringAttribute{Computed: true},
						"network_address": schema.StringAttribute{Computed: true},
						"is_public_zone":  schema.BoolAttribute{Computed: true},
						"description":     schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *SubnetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *SubnetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg SubnetsDataSourceModel
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
	}
	apiResp, diags := callAPI(ctx, d.client, pathSubnetList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := decodeSubnetList(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("decode subnet list", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.Subnets = make([]SubnetItem, 0, len(items))
	for _, raw := range items {
		// Real API list: "id" may be number or string; asIDString handles both.
		// "vttSubnetId" is integer fallback used by fake-api.
		subnetID := asIDString(raw, "id")
		if subnetID == "" {
			subnetID = asIDString(raw, "vttSubnetId")
		}
		// Real API uses "isPublic"; fake-api / spec used "isPublicZone".
		isPublic := asBool(raw, "isPublic")
		if _, ok := raw["isPublicZone"]; ok && !isPublic {
			isPublic = asBool(raw, "isPublicZone")
		}
		cfg.Subnets = append(cfg.Subnets, SubnetItem{
			ID:             types.StringValue(subnetID),
			Name:           types.StringValue(asString(raw, "name")),
			NetworkAddress: types.StringValue(asString(raw, "networkAddress")),
			IsPublicZone:   types.BoolValue(isPublic),
			Description:    types.StringValue(asString(raw, "description")),
		})
	}

	if len(cfg.Subnets) >= listWarningThreshold {
		resp.Diagnostics.AddWarning(
			"Subnet list may be truncated",
			"Returned 1000 or more subnets; the API may have applied a default page size. Consider narrowing by vpc_id.",
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
