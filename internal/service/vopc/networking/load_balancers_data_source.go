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
	_ datasource.DataSource              = (*LoadBalancersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*LoadBalancersDataSource)(nil)
)

// LoadBalancersDataSource implements `data "viettelidc_ovpc_load_balancers"`.
//
// viettelidc_ovpc_load_balancer already reads loadbalancer/list and then keeps
// exactly one item; this returns the whole page, for the cases where the name
// is not known up front — picking a pool id for an autoscale group, say.
type LoadBalancersDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type LoadBalancersDataSourceModel struct {
	VpcID         types.String        `tfsdk:"vpc_id"`
	LoadBalancers []LoadBalancersItem `tfsdk:"load_balancers"`
}

type LoadBalancersItem struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	SubnetID             types.String `tfsdk:"subnet_id"`
	IPAddress            types.String `tfsdk:"ip_address"`
	LoadBalancerType     types.String `tfsdk:"loadbalancer_type"`
	PackageType          types.String `tfsdk:"package_type"`
	Status               types.String `tfsdk:"status"`
	OperatingStatus      types.String `tfsdk:"operating_status"`
	ProvisioningStatus   types.String `tfsdk:"provisioning_status"`
	IsPublicLoadBalancer types.Bool   `tfsdk:"is_public_loadbalancer"`
	AdminStateUp         types.Bool   `tfsdk:"admin_state_up"`
}

func NewLoadBalancersDataSource() datasource.DataSource { return &LoadBalancersDataSource{} }

func (d *LoadBalancersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_load_balancers"
}

func (d *LoadBalancersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the Load Balancers in a VPC. Use viettelidc_ovpc_load_balancer " +
			"to fetch one by id, name or ip_address along with its listeners and pools.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Description: "VPC to list; falls back to the provider default vpc_id.",
			},
			"load_balancers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     schema.StringAttribute{Computed: true, Description: "Load Balancer ID (vttLoadBalancerId)."},
						"name":                   schema.StringAttribute{Computed: true, Description: "Load Balancer name."},
						"subnet_id":              schema.StringAttribute{Computed: true, Description: "Subnet the Load Balancer sits in."},
						"ip_address":             schema.StringAttribute{Computed: true, Description: "Private IP the Load Balancer listens on."},
						"loadbalancer_type":      schema.StringAttribute{Computed: true, Description: `Type, e.g. "NETWORK TCP-UDP".`},
						"package_type":           schema.StringAttribute{Computed: true, Description: `Package, e.g. "LB Compact".`},
						"status":                 schema.StringAttribute{Computed: true, Description: "Current status."},
						"operating_status":       schema.StringAttribute{Computed: true, Description: "Operating status."},
						"provisioning_status":    schema.StringAttribute{Computed: true, Description: "Provisioning status."},
						"is_public_loadbalancer": schema.BoolAttribute{Computed: true, Description: "Whether it is reachable from the internet."},
						"admin_state_up":         schema.BoolAttribute{Computed: true, Description: "Administrative state."},
					},
				},
			},
		},
	}
}

func (d *LoadBalancersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LoadBalancersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg LoadBalancersDataSourceModel
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
		"filters":     []interface{}{},
	}
	apiResp, diags := callAPI(ctx, d.client, pathLoadBalancerList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listResp struct {
		Items []struct {
			VttLoadBalancerID       int64  `json:"vttLoadBalancerId"`
			Name                    string `json:"name"`
			VttSubnetID             int64  `json:"vttSubnetId"`
			IPAddress               string `json:"ipAddress"`
			VttLoadbalancerTypeName string `json:"vttLoadbalancerTypeName"`
			LoadbalancerTypeName    string `json:"loadbalancerTypeName"`
			Status                  string `json:"status"`
			OperatingStatus         string `json:"operatingStatus"`
			ProvisioningStatus      string `json:"provisioningStatus"`
			IsPublicLoadbalancer    bool   `json:"isPublicLoadbalancer"`
			AdminStateUp            bool   `json:"adminStateUp"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	cfg.VpcID = types.StringValue(vpcID)
	cfg.LoadBalancers = make([]LoadBalancersItem, 0, len(listResp.Items))
	for _, it := range listResp.Items {
		cfg.LoadBalancers = append(cfg.LoadBalancers, LoadBalancersItem{
			ID:                   types.StringValue(fmt.Sprintf("%d", it.VttLoadBalancerID)),
			Name:                 types.StringValue(it.Name),
			SubnetID:             types.StringValue(fmt.Sprintf("%d", it.VttSubnetID)),
			IPAddress:            types.StringValue(it.IPAddress),
			LoadBalancerType:     types.StringValue(it.VttLoadbalancerTypeName),
			PackageType:          types.StringValue(it.LoadbalancerTypeName),
			Status:               types.StringValue(it.Status),
			OperatingStatus:      types.StringValue(it.OperatingStatus),
			ProvisioningStatus:   types.StringValue(it.ProvisioningStatus),
			IsPublicLoadBalancer: types.BoolValue(it.IsPublicLoadbalancer),
			AdminStateUp:         types.BoolValue(it.AdminStateUp),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
