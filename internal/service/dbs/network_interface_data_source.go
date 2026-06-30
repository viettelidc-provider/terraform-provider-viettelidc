// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

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
	_ datasource.DataSource              = (*VDBSNetworkInterfaceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSNetworkInterfaceDataSource)(nil)
)

// VDBSNetworkInterfaceDataSource implements `data "viettelidc_vdbs_network_interface"`.
type VDBSNetworkInterfaceDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// VDBSNetworkInterfaceDataSourceModel mirrors the data source schema.
type VDBSNetworkInterfaceDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Status            types.String `tfsdk:"status"`
	IPAddress         types.String `tfsdk:"ip_address"`
	DBSubnetGroupName types.String `tfsdk:"db_subnet_group_name"`
	VpcID             types.String `tfsdk:"vpc_id"`
}

// NewVDBSNetworkInterfaceDataSource constructs the data source.
func NewVDBSNetworkInterfaceDataSource() datasource.DataSource {
	return &VDBSNetworkInterfaceDataSource{}
}

func (d *VDBSNetworkInterfaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_network_interface"
}

func (d *VDBSNetworkInterfaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VDBS Network Interface by id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "DBS Network Interface ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID the network interface belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Network interface name.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Network interface status (e.g. ACTIVE, ATTACHED).",
			},
			"ip_address": schema.StringAttribute{
				Computed:    true,
				Description: "Private IP address assigned to the network interface.",
			},
			"db_subnet_group_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the DBS subnet group this ENI belongs to.",
			},
		},
	}
}

func (d *VDBSNetworkInterfaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *VDBSNetworkInterfaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSNetworkInterfaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   200,
		"filters":    []interface{}{},
		"selected":   6,
		"hostId":     6,
		"customerId": d.customerID,
		"planType":   "dbs",
	}

	apiResp, diags := callAPI(ctx, d.client, pathDBSNICList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS network interface not found",
			fmt.Sprintf("DBS network interface not found with id %s (no list data)", cfg.ID.ValueString()),
		)
		return
	}

	var listData map[string]interface{}
	raw, _ := json.Marshal(apiResp.Data)
	json.Unmarshal(raw, &listData)
	items, ok := listData["items"]
	if !ok {
		resp.Diagnostics.AddError(
			"DBS network interface not found",
			fmt.Sprintf("DBS network interface not found with id %s (no items in list)", cfg.ID.ValueString()),
		)
		return
	}

	m := filterListByID(items, cfg.ID.ValueString())
	if m == nil {
		resp.Diagnostics.AddError(
			"DBS network interface not found",
			fmt.Sprintf("DBS network interface not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	cfg.Name = types.StringValue(asString(m, "name"))
	cfg.Status = types.StringValue(asString(m, "status"))
	cfg.IPAddress = types.StringValue(asString(m, "ipAddress"))
	cfg.DBSubnetGroupName = types.StringValue(asString(m, "dbSubnetGroupName"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
