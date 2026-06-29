// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

// ─── 1. SINGLE BACKUP RECORD DATA SOURCE ──────────────────────────────────────

type backupRecordDataSource struct {
	clientData *providerdata.ProviderData
}

type BackupRecordDSModel struct {
	ID         types.String `tfsdk:"id"`
	HostID     types.Int64  `tfsdk:"host_id"`
	PlanType   types.String `tfsdk:"plan_type"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	CreatedAt  types.String `tfsdk:"created_at"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeID   types.String `tfsdk:"volume_id"`
	VolumeName types.String `tfsdk:"volume_name"`
	VolumeSize types.Int64  `tfsdk:"volume_size"`
}

func NewBackupRecordDataSource() datasource.DataSource {
	return &backupRecordDataSource{}
}

func (d *backupRecordDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_backup_record"
}

func (d *backupRecordDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *backupRecordDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source to get detail of a VKS manual backup record.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the manual backup record.",
				Required:    true,
			},
			"host_id": schema.Int64Attribute{
				Description: "ID of the host.",
				Required:    true,
			},
			"plan_type": schema.StringAttribute{
				Description: "Plan type, default is 'k8s'.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"size": schema.Int64Attribute{
				Computed: true,
			},
			"volume_id": schema.StringAttribute{
				Computed: true,
			},
			"volume_name": schema.StringAttribute{
				Computed: true,
			},
			"volume_size": schema.Int64Attribute{
				Computed: true,
			},
		},
	}
}

func (d *backupRecordDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BackupRecordDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !config.PlanType.IsNull() && !config.PlanType.IsUnknown() {
		planType = config.PlanType.ValueString()
	}

	payload := map[string]interface{}{
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
		"size":        0,
		"sorts":       []interface{}{},
		"host_id":     config.HostID.ValueInt64(),
		"customer_id": d.clientData.CustomerID,
		"planType":    planType,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathK8sSchedulerBackupList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse backup list-paging response")
		return
	}

	itemsVal, ok := responseMap["items"]
	if !ok || itemsVal == nil {
		resp.Diagnostics.AddError("Not Found", "No backups found")
		return
	}

	itemsList, ok := itemsVal.([]interface{})
	if !ok {
		resp.Diagnostics.AddError("Not Found", "No backups found")
		return
	}

	targetID := config.ID.ValueString()
	var foundItem map[string]interface{}
	for _, itemRaw := range itemsList {
		if itemMap, ok := itemRaw.(map[string]interface{}); ok {
			itemID := asString(itemMap, "id")
			if itemID == targetID {
				foundItem = itemMap
				break
			}
		}
	}

	if foundItem == nil {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Backup record with ID %s not found", targetID))
		return
	}

	config.Name = types.StringValue(asString(foundItem, "name"))
	config.Status = types.StringValue(asString(foundItem, "status"))
	config.Size = types.Int64Value(asInt64(foundItem, "size"))
	config.PlanType = types.StringValue(planType)

	createdAtRaw := asString(foundItem, "createdAt")
	if strings.Contains(createdAtRaw, ".") {
		createdAtRaw = strings.Split(createdAtRaw, ".")[0]
	}
	config.CreatedAt = types.StringValue(createdAtRaw)

	config.VolumeID = types.StringValue(asString(foundItem, "blockStorageId"))
	config.VolumeName = types.StringValue(asString(foundItem, "blockStorageName"))
	config.VolumeSize = types.Int64Value(asInt64(foundItem, "blockStorageSize"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ─── 2. PLURAL BACKUP RECORDS DATA SOURCE ─────────────────────────────────────

type backupRecordsDataSource struct {
	clientData *providerdata.ProviderData
}

type BackupRecordsDSModel struct {
	ID       types.String            `tfsdk:"id"`
	HostID   types.Int64             `tfsdk:"host_id"`
	PlanType types.String            `tfsdk:"plan_type"`
	Backups  []BackupRecordItemModel `tfsdk:"backups"`
}

type BackupRecordItemModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Status     types.String `tfsdk:"status"`
	CreatedAt  types.String `tfsdk:"created_at"`
	Size       types.Int64  `tfsdk:"size"`
	VolumeID   types.String `tfsdk:"volume_id"`
	VolumeName types.String `tfsdk:"volume_name"`
	VolumeSize types.Int64  `tfsdk:"volume_size"`
}

func NewBackupRecordsDataSource() datasource.DataSource {
	return &backupRecordsDataSource{}
}

func (d *backupRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_backup_records"
}

func (d *backupRecordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *backupRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source to list all VKS backup records (manual and automatic) of a host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"host_id": schema.Int64Attribute{
				Description: "ID of the host.",
				Required:    true,
			},
			"plan_type": schema.StringAttribute{
				Description: "Plan type, default is 'k8s'.",
				Optional:    true,
				Computed:    true,
			},
			"backups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true},
						"name":        schema.StringAttribute{Computed: true},
						"status":      schema.StringAttribute{Computed: true},
						"created_at":  schema.StringAttribute{Computed: true},
						"size":        schema.Int64Attribute{Computed: true},
						"volume_id":   schema.StringAttribute{Computed: true},
						"volume_name": schema.StringAttribute{Computed: true},
						"volume_size": schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *backupRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BackupRecordsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !config.PlanType.IsNull() && !config.PlanType.IsUnknown() {
		planType = config.PlanType.ValueString()
	}

	payload := map[string]interface{}{
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
		"size":        0,
		"sorts":       []interface{}{},
		"host_id":     config.HostID.ValueInt64(),
		"customer_id": d.clientData.CustomerID,
		"planType":    planType,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathK8sSchedulerBackupList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse backups list-paging response")
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("%d/backups", config.HostID.ValueInt64()))
	config.PlanType = types.StringValue(planType)
	config.Backups = []BackupRecordItemModel{}

	itemsVal, ok := responseMap["items"]
	if ok && itemsVal != nil {
		if itemsList, ok := itemsVal.([]interface{}); ok {
			for _, itemRaw := range itemsList {
				if itemMap, ok := itemRaw.(map[string]interface{}); ok {
					createdAtRaw := asString(itemMap, "createdAt")
					if strings.Contains(createdAtRaw, ".") {
						createdAtRaw = strings.Split(createdAtRaw, ".")[0]
					}

					item := BackupRecordItemModel{
						ID:         types.StringValue(asString(itemMap, "id")),
						Name:       types.StringValue(asString(itemMap, "name")),
						Status:     types.StringValue(asString(itemMap, "status")),
						CreatedAt:  types.StringValue(createdAtRaw),
						Size:       types.Int64Value(asInt64(itemMap, "size")),
						VolumeID:   types.StringValue(asString(itemMap, "blockStorageId")),
						VolumeName: types.StringValue(asString(itemMap, "blockStorageName")),
						VolumeSize: types.Int64Value(asInt64(itemMap, "blockStorageSize")),
					}
					config.Backups = append(config.Backups, item)
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
