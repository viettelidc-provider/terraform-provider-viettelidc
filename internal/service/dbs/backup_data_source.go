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
	_ datasource.DataSource              = (*VDBSBackupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSBackupDataSource)(nil)
)

func NewVDBSBackupDataSource() datasource.DataSource {
	return &VDBSBackupDataSource{}
}

type VDBSBackupDataSource struct {
	client     *client.Client
	customerID string
}

type VDBSBackupDataSourceModel struct {
	ID                types.String  `tfsdk:"id"`
	Name              types.String  `tfsdk:"name"`
	Status            types.String  `tfsdk:"status"`
	Description       types.String  `tfsdk:"description"`
	InstanceID        types.String  `tfsdk:"instance_id"`
	InstanceName      types.String  `tfsdk:"instance_name"`
	BlockStorageSize  types.Int64   `tfsdk:"block_storage_size_gb"`
	BlockStorageUsed  types.Float64 `tfsdk:"block_storage_used_gb"`
	ObjectStorageSize types.Int64   `tfsdk:"object_storage_size_gb"`
	ObjectStorageUsed types.Float64 `tfsdk:"object_storage_used_gb"`
	VpcID             types.Int64   `tfsdk:"vpc_id"`
	VpcName           types.String  `tfsdk:"vpc_name"`
	Threshold         types.Int64   `tfsdk:"threshold"`
	ServiceInitID     types.Int64   `tfsdk:"service_init_id"`
}

func (d *VDBSBackupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_backup"
}

func (d *VDBSBackupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Backup ID (UUID)",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Backup name",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Backup status (ACTIVE, INACTIVE, etc)",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Backup description",
			},
			"instance_id": schema.StringAttribute{
				Computed:    true,
				Description: "Related database instance ID",
			},
			"instance_name": schema.StringAttribute{
				Computed:    true,
				Description: "Related database instance name",
			},
			"block_storage_size_gb": schema.Int64Attribute{
				Computed:    true,
				Description: "Block storage size in GB",
			},
			"block_storage_used_gb": schema.Float64Attribute{
				Computed:    true,
				Description: "Block storage used in GB",
			},
			"object_storage_size_gb": schema.Int64Attribute{
				Computed:    true,
				Description: "Object storage size in GB",
			},
			"object_storage_used_gb": schema.Float64Attribute{
				Computed:    true,
				Description: "Object storage used in GB",
			},
			"vpc_id": schema.Int64Attribute{
				Computed:    true,
				Description: "VPC ID",
			},
			"vpc_name": schema.StringAttribute{
				Computed:    true,
				Description: "VPC name",
			},
			"threshold": schema.Int64Attribute{
				Computed:    true,
				Description: "Backup threshold percentage",
			},
			"service_init_id": schema.Int64Attribute{
				Computed:    true,
				Description: "Service init ID",
			},
		},
	}
}

func (d *VDBSBackupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
}

func (d *VDBSBackupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSBackupDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call API to list backups and filter by ID
	body := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   100,
		"filters":    []interface{}{},
		"hostId":     6,
		"customerId": d.customerID,
		"planType":   "dbs",
	}

	apiResp, diags := callAPI(ctx, d.client, pathBackupList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"DBS backup not found",
			fmt.Sprintf("DBS backup not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS backup not found",
			fmt.Sprintf("DBS backup not found with id %s", cfg.ID.ValueString()),
		)
		return
	}

	// Parse response
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

	// Extract items array
	var backups []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					backups = append(backups, itemMap)
				}
			}
		}
	}

	// Find backup by ID
	backupID := cfg.ID.ValueString()
	var foundBackup map[string]interface{}
	for _, backup := range backups {
		if asString(backup, "id") == backupID {
			foundBackup = backup
			break
		}
	}

	if foundBackup == nil {
		resp.Diagnostics.AddError(
			"DBS backup not found",
			fmt.Sprintf("No backup found with id %s", backupID),
		)
		return
	}

	// Map response to model
	cfg.ID = types.StringValue(asString(foundBackup, "id"))
	cfg.Name = types.StringValue(asString(foundBackup, "name"))
	cfg.Status = types.StringValue(asString(foundBackup, "status"))
	cfg.Description = types.StringValue(asString(foundBackup, "description"))
	cfg.InstanceID = types.StringValue(asString(foundBackup, "instanceId"))
	cfg.InstanceName = types.StringValue(asString(foundBackup, "instanceName"))
	cfg.VpcID = types.Int64Value(asInt64(foundBackup, "vpcId"))
	cfg.VpcName = types.StringValue(asString(foundBackup, "vpcName"))
	cfg.Threshold = types.Int64Value(asInt64(foundBackup, "threshold"))
	cfg.ServiceInitID = types.Int64Value(asInt64(foundBackup, "serviceInitId"))

	// Parse block storage
	if blockStorageRaw, ok := foundBackup["blockStorage"].(map[string]interface{}); ok {
		cfg.BlockStorageSize = types.Int64Value(asInt64(blockStorageRaw, "sizeInGB"))
		if usedRaw, ok := blockStorageRaw["usedInGB"].(float64); ok {
			cfg.BlockStorageUsed = types.Float64Value(usedRaw)
		}
	}

	// Parse object storage
	if objectStorageRaw, ok := foundBackup["objectStorage"].(map[string]interface{}); ok {
		cfg.ObjectStorageSize = types.Int64Value(asInt64(objectStorageRaw, "sizeInGB"))
		if usedRaw, ok := objectStorageRaw["usedInGB"].(float64); ok {
			cfg.ObjectStorageUsed = types.Float64Value(usedRaw)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
