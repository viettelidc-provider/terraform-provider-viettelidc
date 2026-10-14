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
	_ datasource.DataSource              = (*L7AvailablePoolsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*L7AvailablePoolsDataSource)(nil)
)

type L7AvailablePoolsDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7AvailablePoolsDataSource() datasource.DataSource {
	return &L7AvailablePoolsDataSource{}
}

type L7AvailablePoolsDataSourceModel struct {
	ID         types.String        `tfsdk:"id"`
	VpcID      types.Int64         `tfsdk:"vpc_id"`
	ListenerID types.Int64         `tfsdk:"listener_id"`
	Pools      []AvailablePoolItem `tfsdk:"pools"`
}

type AvailablePoolItem struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *L7AvailablePoolsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_available_pools"
}

func (d *L7AvailablePoolsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing available and protocol-compatible pools for an L7 policy REDIRECT_TO_POOL action.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier for this data source reading (<vpc_id>/<listener_id>).",
			},
			"vpc_id": schema.Int64Attribute{
				Required:    true,
				Description: "VPC ID owning the Load Balancer.",
			},
			"listener_id": schema.Int64Attribute{
				Required:    true,
				Description: "Listener ID to list compatible available pools for.",
			},
			"pools": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of available and compatible pools.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:    true,
							Description: "Pool ID.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Pool Name.",
						},
					},
				},
			},
		},
	}
}

func (d *L7AvailablePoolsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *L7AvailablePoolsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg L7AvailablePoolsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := cfg.VpcID.ValueInt64()
	listenerID := cfg.ListenerID.ValueInt64()

	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                d.customerID,
		"customer_id":               d.customerID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
	}

	apiResp, diags := callAPI(ctx, d.client, pathL7PolicyPools, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rawPools []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &rawPools); err != nil {
			resp.Diagnostics.AddError("Parse Available Pools Failed", err.Error())
			return
		}
	}

	pools := make([]AvailablePoolItem, 0, len(rawPools))
	for _, p := range rawPools {
		pools = append(pools, AvailablePoolItem{
			ID:   types.Int64Value(p.ID),
			Name: types.StringValue(p.Name),
		})
	}

	cfg.ID = types.StringValue(fmt.Sprintf("%d/%d", vpcID, listenerID))
	cfg.Pools = pools
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
