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
	_ datasource.DataSource              = (*VDBSSubnetGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSSubnetGroupDataSource)(nil)
)

// VDBSSubnetGroupDataSource implements `data "viettelidc_vdbs_subnet_group"`.
type VDBSSubnetGroupDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
	hostID       int64
}

// VDBSSubnetGroupDataSourceModel mirrors the data source schema.
type VDBSSubnetGroupDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	SubnetIDs types.List   `tfsdk:"subnet_ids"`
	VpcID     types.String `tfsdk:"vpc_id"`
}

// NewVDBSSubnetGroupDataSource constructs the data source.
func NewVDBSSubnetGroupDataSource() datasource.DataSource {
	return &VDBSSubnetGroupDataSource{}
}

func (d *VDBSSubnetGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_subnet_group"
}

func (d *VDBSSubnetGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VDBS Subnet Group by id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Subnet group ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID the subnet group belongs to.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Subnet group name.",
			},
			"subnet_ids": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "List of subnet IDs in the group.",
			},
		},
	}
}

func (d *VDBSSubnetGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
	d.hostID = pd.HostID
}

func (d *VDBSSubnetGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSSubnetGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := cfg.VpcID.ValueString()
	if vpcID == "" {
		vpcID = d.defaultVpcID
	}

	// Call list endpoint to get all subnet groups
	body := map[string]interface{}{
		"page_index":  0,
		"page_size":   200,
		"filters":     []interface{}{},
		"selected":    d.hostID,
		"host_id":     d.hostID,
		"customer_id": d.customerID,
		"vpc_id":      vpcID,
		"plan_type":   "dbs",
	}

	apiResp, diags := callAPI(ctx, d.client, pathSubnetGroupList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS subnet group not found",
			"DBS subnet group not found (empty response)",
		)
		return
	}

	// Parse list response to extract items array
	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}
	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		resp.Diagnostics.AddError("decode error", err.Error())
		return
	}

	var m map[string]interface{}
	if !cfg.ID.IsNull() && !cfg.ID.IsUnknown() && cfg.ID.ValueString() != "" {
		m = filterListByID(listData["items"], cfg.ID.ValueString())
		if m == nil {
			resp.Diagnostics.AddError(
				"DBS subnet group not found",
				fmt.Sprintf("DBS subnet group not found with id %s", cfg.ID.ValueString()),
			)
			return
		}
	} else if !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != "" {
		m = filterListByName(listData["items"], cfg.Name.ValueString())
		if m == nil {
			resp.Diagnostics.AddError(
				"DBS subnet group not found",
				fmt.Sprintf("DBS subnet group not found with name %s", cfg.Name.ValueString()),
			)
			return
		}
	} else {
		resp.Diagnostics.AddError(
			"Invalid Query",
			"Must specify either id or name to query DBS subnet group",
		)
		return
	}

	cfg.ID = types.StringValue(asIDString(m, "id"))
	cfg.Name = types.StringValue(asString(m, "name"))
	if vpcIDResp := asIDString(m, "vpcId"); vpcIDResp != "" {
		cfg.VpcID = types.StringValue(vpcIDResp)
	}

	// subnetIds is a JSON array in the response (camelCase).
	subnetIDs, listDiags := listFromJSONArray(ctx, m, "subnetIds")
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg.SubnetIDs = subnetIDs

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
