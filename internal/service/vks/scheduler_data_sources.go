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

// ─── 1. SINGLE SCHEDULER DATA SOURCE ──────────────────────────────────────────

type schedulerDataSource struct {
	clientData *providerdata.ProviderData
}

type SchedulerDSModel struct {
	ID         types.String `tfsdk:"id"`
	HostID     types.Int64  `tfsdk:"host_id"`
	PlanType   types.String `tfsdk:"plan_type"`
	Name       types.String `tfsdk:"name"`
	StartTime  types.String `tfsdk:"start_time"`
	FinishTime types.String `tfsdk:"finish_time"`
	Cycle      types.Int64  `tfsdk:"cycle"`
	Unit       types.String `tfsdk:"unit"`
	Quantity   types.Int64  `tfsdk:"quantity"`
	Status     types.String `tfsdk:"status"`
}

func NewSchedulerDataSource() datasource.DataSource {
	return &schedulerDataSource{}
}

func (d *schedulerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_scheduler"
}

func (d *schedulerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *schedulerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source to get detail of a VKS backup scheduler.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the backup scheduler.",
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
			"start_time": schema.StringAttribute{
				Computed: true,
			},
			"finish_time": schema.StringAttribute{
				Computed: true,
			},
			"cycle": schema.Int64Attribute{
				Computed: true,
			},
			"unit": schema.StringAttribute{
				Computed: true,
			},
			"quantity": schema.Int64Attribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *schedulerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchedulerDSModel
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

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathK8sSchedulerListPaging, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse scheduler list-paging response")
		return
	}

	itemsVal, ok := responseMap["items"]
	if !ok || itemsVal == nil {
		resp.Diagnostics.AddError("Not Found", "No schedules found")
		return
	}

	itemsList, ok := itemsVal.([]interface{})
	if !ok {
		resp.Diagnostics.AddError("Not Found", "No schedules found")
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
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Backup schedule with ID %s not found", targetID))
		return
	}

	config.Name = types.StringValue(asString(foundItem, "name"))
	config.Status = types.StringValue(asString(foundItem, "status"))

	startDateRaw := asString(foundItem, "startDate")
	if strings.Contains(startDateRaw, ".") {
		startDateRaw = strings.Split(startDateRaw, ".")[0]
	}
	config.StartTime = types.StringValue(startDateRaw)

	finishAtRaw := asString(foundItem, "finishAt")
	if strings.Contains(finishAtRaw, ".") {
		finishAtRaw = strings.Split(finishAtRaw, ".")[0]
	}
	config.FinishTime = types.StringValue(finishAtRaw)

	config.Quantity = types.Int64Value(asInt64(foundItem, "quantityCycle"))
	config.Unit = types.StringValue(asString(foundItem, "unitCycle"))
	config.PlanType = types.StringValue(planType)

	backupCycle := asInt64(foundItem, "backupCycle")
	unitCycle := asString(foundItem, "unitCycle")
	var cycleSec int64 = 86400
	if unitCycle == "day" {
		cycleSec = backupCycle * 86400
	} else if unitCycle == "hour" {
		cycleSec = backupCycle * 3600
	} else if unitCycle == "week" {
		cycleSec = backupCycle * 86400 * 7
	} else {
		cycleSec = backupCycle
	}
	config.Cycle = types.Int64Value(cycleSec)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ─── 2. LIST SCHEDULERS DATA SOURCE ─────────────────────────────────────────

type schedulersDataSource struct {
	clientData *providerdata.ProviderData
}

type SchedulersDSModel struct {
	ID         types.String         `tfsdk:"id"`
	HostID     types.Int64          `tfsdk:"host_id"`
	PlanType   types.String         `tfsdk:"plan_type"`
	Schedulers []SchedulerItemModel `tfsdk:"schedulers"`
}

type SchedulerItemModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	StartTime  types.String `tfsdk:"start_time"`
	FinishTime types.String `tfsdk:"finish_time"`
	Cycle      types.Int64  `tfsdk:"cycle"`
	Unit       types.String `tfsdk:"unit"`
	Quantity   types.Int64  `tfsdk:"quantity"`
	Status     types.String `tfsdk:"status"`
}

func NewSchedulersDataSource() datasource.DataSource {
	return &schedulersDataSource{}
}

func (d *schedulersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_schedulers"
}

func (d *schedulersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *schedulersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source to list all VKS backup schedulers of a host.",
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
			"schedulers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true},
						"name":        schema.StringAttribute{Computed: true},
						"start_time":  schema.StringAttribute{Computed: true},
						"finish_time": schema.StringAttribute{Computed: true},
						"cycle":       schema.Int64Attribute{Computed: true},
						"unit":        schema.StringAttribute{Computed: true},
						"quantity":    schema.Int64Attribute{Computed: true},
						"status":      schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *schedulersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchedulersDSModel
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

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathK8sSchedulerListPaging, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse schedulers list-paging response")
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("%d/schedulers", config.HostID.ValueInt64()))
	config.PlanType = types.StringValue(planType)
	config.Schedulers = []SchedulerItemModel{}

	itemsVal, ok := responseMap["items"]
	if ok && itemsVal != nil {
		if itemsList, ok := itemsVal.([]interface{}); ok {
			for _, itemRaw := range itemsList {
				if itemMap, ok := itemRaw.(map[string]interface{}); ok {
					startDateRaw := asString(itemMap, "startDate")
					if strings.Contains(startDateRaw, ".") {
						startDateRaw = strings.Split(startDateRaw, ".")[0]
					}
					finishAtRaw := asString(itemMap, "finishAt")
					if strings.Contains(finishAtRaw, ".") {
						finishAtRaw = strings.Split(finishAtRaw, ".")[0]
					}

					backupCycle := asInt64(itemMap, "backupCycle")
					unitCycle := asString(itemMap, "unitCycle")
					var cycleSec int64 = 86400
					if unitCycle == "day" {
						cycleSec = backupCycle * 86400
					} else if unitCycle == "hour" {
						cycleSec = backupCycle * 3600
					} else if unitCycle == "week" {
						cycleSec = backupCycle * 86400 * 7
					} else {
						cycleSec = backupCycle
					}

					item := SchedulerItemModel{
						ID:         types.StringValue(asString(itemMap, "id")),
						Name:       types.StringValue(asString(itemMap, "name")),
						Status:     types.StringValue(asString(itemMap, "status")),
						StartTime:  types.StringValue(startDateRaw),
						FinishTime: types.StringValue(finishAtRaw),
						Quantity:   types.Int64Value(asInt64(itemMap, "quantityCycle")),
						Unit:       types.StringValue(unitCycle),
						Cycle:      types.Int64Value(cycleSec),
					}
					config.Schedulers = append(config.Schedulers, item)
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ─── 3. BACKUP HISTORY DATA SOURCE ──────────────────────────────────────────

type schedulerBackupsDataSource struct {
	clientData *providerdata.ProviderData
}

type SchedulerBackupsDSModel struct {
	ID       types.String      `tfsdk:"id"`
	HostID   types.Int64       `tfsdk:"host_id"`
	PlanType types.String      `tfsdk:"plan_type"`
	Backups  []BackupItemModel `tfsdk:"backups"`
}

type BackupItemModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
	Size           types.Int64  `tfsdk:"size"`
	BlockStorageID types.String `tfsdk:"block_storage_id"`
}

func NewSchedulerBackupsDataSource() datasource.DataSource {
	return &schedulerBackupsDataSource{}
}

func (d *schedulerBackupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_scheduler_backups"
}

func (d *schedulerBackupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *schedulerBackupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source to list all VKS backups created by schedulers.",
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
						"id":               schema.StringAttribute{Computed: true},
						"name":             schema.StringAttribute{Computed: true},
						"status":           schema.StringAttribute{Computed: true},
						"created_at":       schema.StringAttribute{Computed: true},
						"size":             schema.Int64Attribute{Computed: true},
						"block_storage_id": schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *schedulerBackupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchedulerBackupsDSModel
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

	config.ID = types.StringValue(fmt.Sprintf("%d/backups", config.HostID.ValueInt64()))
	config.PlanType = types.StringValue(planType)
	config.Backups = []BackupItemModel{}

	itemsVal, ok := responseMap["items"]
	if ok && itemsVal != nil {
		if itemsList, ok := itemsVal.([]interface{}); ok {
			for _, itemRaw := range itemsList {
				if itemMap, ok := itemRaw.(map[string]interface{}); ok {
					createdAtRaw := asString(itemMap, "createdAt")
					if strings.Contains(createdAtRaw, ".") {
						createdAtRaw = strings.Split(createdAtRaw, ".")[0]
					}

					item := BackupItemModel{
						ID:             types.StringValue(asString(itemMap, "id")),
						Name:           types.StringValue(asString(itemMap, "name")),
						Status:         types.StringValue(asString(itemMap, "status")),
						CreatedAt:      types.StringValue(createdAtRaw),
						Size:           types.Int64Value(asInt64(itemMap, "size")),
						BlockStorageID: types.StringValue(asString(itemMap, "blockStorageId")),
					}
					config.Backups = append(config.Backups, item)
				}
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
