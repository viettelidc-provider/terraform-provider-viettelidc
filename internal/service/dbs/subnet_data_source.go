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
	_ datasource.DataSource              = (*VDBSSubnetDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSSubnetDataSource)(nil)
)

// VDBSSubnetDataSource implements `data "viettelidc_vdbs_subnet"`.
type VDBSSubnetDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// VDBSSubnetDataSourceModel mirrors the data source schema.
type VDBSSubnetDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Status           types.String `tfsdk:"status"`
	CIDR             types.String `tfsdk:"cidr"`
	VpcID            types.String `tfsdk:"vpc_id"`
	AvailabilityZone types.String `tfsdk:"availability_zone"`
}

// NewVDBSSubnetDataSource constructs the data source.
func NewVDBSSubnetDataSource() datasource.DataSource {
	return &VDBSSubnetDataSource{}
}

func (d *VDBSSubnetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_subnet"
}

func (d *VDBSSubnetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VDBS Subnet by id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "DBS Subnet ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID the subnet belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Subnet name.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Subnet status (e.g. ACTIVE).",
			},
			"cidr": schema.StringAttribute{
				Computed:    true,
				Description: "CIDR block of the subnet.",
			},
			"availability_zone": schema.StringAttribute{
				Computed:    true,
				Description: "Availability zone of the subnet.",
			},
		},
	}
}

func (d *VDBSSubnetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *VDBSSubnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSSubnetDataSourceModel
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

	apiResp, diags := callAPI(ctx, d.client, pathDBSSubnetList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS subnet not found",
			fmt.Sprintf("DBS subnet not found with id %s (no list data)", cfg.ID.ValueString()),
		)
		return
	}

	var listData map[string]interface{}
	raw, _ := json.Marshal(apiResp.Data)
	json.Unmarshal(raw, &listData)
	items, ok := listData["items"]
	if !ok {
		resp.Diagnostics.AddError(
			"DBS subnet not found",
			fmt.Sprintf("DBS subnet not found with id %s (no items in list)", cfg.ID.ValueString()),
		)
		return
	}

	m := filterListByID(items, cfg.ID.ValueString())
	if m == nil {
		resp.Diagnostics.AddError(
			"DBS subnet not found",
			fmt.Sprintf("DBS subnet not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	cfg.Name = types.StringValue(asString(m, "name"))
	cfg.Status = types.StringValue(asString(m, "status"))
	cfg.CIDR = types.StringValue(asString(m, "cidr"))
	cfg.AvailabilityZone = types.StringValue(asString(m, "availabilityZone"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
