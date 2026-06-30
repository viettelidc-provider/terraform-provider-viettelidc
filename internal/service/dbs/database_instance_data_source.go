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
	_ datasource.DataSource              = (*VDBSDatabaseInstanceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSDatabaseInstanceDataSource)(nil)
)

// VDBSDatabaseInstanceDataSource implements `data "viettelidc_vdbs_database_instance"`.
type VDBSDatabaseInstanceDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
	hostID       int64
}

// VDBSDatabaseInstanceDataSourceModel mirrors the data source schema.
type VDBSDatabaseInstanceDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	VpcID              types.String `tfsdk:"vpc_id"`
	Name               types.String `tfsdk:"name"`
	Status             types.String `tfsdk:"status"`
	FlavorID           types.String `tfsdk:"flavor_id"`
	VolumeSize         types.Int64  `tfsdk:"volume_size"`
	DBSubnetGroupName  types.String `tfsdk:"db_subnet_group_name"`
	ParameterGroupName types.String `tfsdk:"parameter_group_name"`
}

// NewVDBSDatabaseInstanceDataSource constructs the data source.
func NewVDBSDatabaseInstanceDataSource() datasource.DataSource {
	return &VDBSDatabaseInstanceDataSource{}
}

func (d *VDBSDatabaseInstanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_database_instance"
}

func (d *VDBSDatabaseInstanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing ViettelIDC VDBS database instance by id.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Database instance ID.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID the database instance belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Instance name.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status (e.g. ACTIVE, ERROR).",
			},
			"flavor_id": schema.StringAttribute{
				Computed:    true,
				Description: "Flavor (instance type) ID.",
			},
			"volume_size": schema.Int64Attribute{
				Computed:    true,
				Description: "Storage volume size in GB.",
			},
			"db_subnet_group_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the subnet group attached to the instance.",
			},
			"parameter_group_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the parameter group attached to the instance.",
			},
		},
	}
}

func (d *VDBSDatabaseInstanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *VDBSDatabaseInstanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSDatabaseInstanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(cfg.VpcID.ValueString(), d.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call list-instance endpoint to get instances
	// Note: API endpoint returns all instances - we filter locally by vttDbaasInstanceId
	body := map[string]interface{}{
		"id":          cfg.ID.ValueString(),
		"host_id":     d.hostID,
		"customer_id": d.customerID,
		"plan_type":   "dbs",
	}

	apiResp, diags := callAPI(ctx, d.client, pathDBInstanceList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"DBS database instance not found",
			fmt.Sprintf("DBS database instance not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS database instance not found",
			fmt.Sprintf("DBS database instance not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	// Parse pagination response object to extract items array
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

	// Extract items array from pagination response
	var instances []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					instances = append(instances, itemMap)
				}
			}
		}
	}

	// Find the instance matching the requested ID
	instanceID := cfg.ID.ValueString()
	var foundInstance map[string]interface{}
	for _, inst := range instances {
		// Check if this instance matches the requested ID
		instID := asString(inst, "id")
		if instID == instanceID {
			foundInstance = inst
			break
		}
	}

	if foundInstance == nil {
		resp.Diagnostics.AddError(
			"DBS database instance not found",
			fmt.Sprintf("DBS database instance not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	// Map response fields to config
	cfg.VpcID = types.StringValue(vpcID)
	cfg.Name = types.StringValue(asString(foundInstance, "name"))
	cfg.Status = types.StringValue(asString(foundInstance, "status"))
	flavorID := asString(foundInstance, "flavorId")
	if flavorID == "" {
		cpu := asInt64(foundInstance, "cpuSize")
		ram := asInt64(foundInstance, "memorySize")
		if cpu == 2 && ram == 2 {
			flavorID = "db.t3.medium"
		}
	}
	cfg.FlavorID = types.StringValue(flavorID)

	volumeSize := asInt64(foundInstance, "volume")
	if volumeSize == 0 {
		volumeSize = asInt64(foundInstance, "storage")
	}
	cfg.VolumeSize = types.Int64Value(volumeSize)
	cfg.DBSubnetGroupName = types.StringValue("")  // Not provided in list response
	cfg.ParameterGroupName = types.StringValue("") // Not provided in list response

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
