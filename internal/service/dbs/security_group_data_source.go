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
	_ datasource.DataSource              = (*VDBSSecurityGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSSecurityGroupDataSource)(nil)
)

// VDBSSecurityGroupDataSource implements `data "viettelidc_vdbs_security_group"`.
type VDBSSecurityGroupDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
	hostID       int64
}

// VDBSSecurityGroupDataSourceModel mirrors the data source schema.
type VDBSSecurityGroupDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

// NewVDBSSecurityGroupDataSource constructs the data source.
func NewVDBSSecurityGroupDataSource() datasource.DataSource {
	return &VDBSSecurityGroupDataSource{}
}

func (d *VDBSSecurityGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_security_group"
}

func (d *VDBSSecurityGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VDBS Security Group by id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Security group ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID the security group belongs to.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Security group name.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Security group description.",
			},
		},
	}
}

func (d *VDBSSecurityGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *VDBSSecurityGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSSecurityGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := cfg.VpcID.ValueString()
	if vpcID == "" {
		vpcID = d.defaultVpcID
	}

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

	apiResp, diags := callAPI(ctx, d.client, pathDBSGList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS security group not found",
			"DBS security group not found (empty response)",
		)
		return
	}

	var listData map[string]interface{}
	raw, _ := json.Marshal(apiResp.Data)
	json.Unmarshal(raw, &listData)
	items, ok := listData["items"]
	if !ok {
		resp.Diagnostics.AddError(
			"DBS security group not found",
			"DBS security group not found (no items in list)",
		)
		return
	}

	var m map[string]interface{}
	if !cfg.ID.IsNull() && !cfg.ID.IsUnknown() && cfg.ID.ValueString() != "" {
		m = filterListByID(items, cfg.ID.ValueString())
		if m == nil {
			resp.Diagnostics.AddError(
				"DBS security group not found",
				fmt.Sprintf("DBS security group not found with id %s", cfg.ID.ValueString()),
			)
			return
		}
	} else if !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != "" {
		m = filterListByName(items, cfg.Name.ValueString())
		if m == nil {
			resp.Diagnostics.AddError(
				"DBS security group not found",
				fmt.Sprintf("DBS security group not found with name %s", cfg.Name.ValueString()),
			)
			return
		}
	} else {
		resp.Diagnostics.AddError(
			"Invalid Query",
			"Must specify either id or name to query DBS security group",
		)
		return
	}

	cfg.ID = types.StringValue(asIDString(m, "id"))
	cfg.Name = types.StringValue(asString(m, "name"))
	cfg.Description = types.StringValue(asString(m, "description"))
	if vpcIDResp := asIDString(m, "vpcId"); vpcIDResp != "" {
		cfg.VpcID = types.StringValue(vpcIDResp)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
