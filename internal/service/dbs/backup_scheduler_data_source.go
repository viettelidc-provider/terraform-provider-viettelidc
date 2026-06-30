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
	_ datasource.DataSource              = (*VDBSBackupSchedulerDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*VDBSBackupSchedulerDataSource)(nil)
)

func NewVDBSBackupSchedulerDataSource() datasource.DataSource {
	return &VDBSBackupSchedulerDataSource{}
}

type VDBSBackupSchedulerDataSource struct {
	client     *client.Client
	customerID string
}

type VDBSBackupSchedulerDataSourceModel struct {
	ID                     types.Int64  `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Status                 types.String `tfsdk:"status"`
	Description            types.String `tfsdk:"description"`
	SchedulerType          types.String `tfsdk:"scheduler_type"`
	Location               types.String `tfsdk:"location"`
	MaxRecord              types.Int64  `tfsdk:"max_record"`
	CurrentRecord          types.Int64  `tfsdk:"current_record"`
	NumberOfInstance       types.Int64  `tfsdk:"number_of_instance"`
	VpcID                  types.Int64  `tfsdk:"vpc_id"`
	FixedRate              types.Int64  `tfsdk:"fixed_rate"`
	PrevSchedulerRunTime   types.String `tfsdk:"prev_scheduler_run_time"`
	PrevSchedulerRunStatus types.String `tfsdk:"prev_scheduler_run_status"`
	NextSchedulerRunTime   types.String `tfsdk:"next_scheduler_run_time"`
	CreateTime             types.String `tfsdk:"create_time"`
	UpdateTime             types.String `tfsdk:"update_time"`
}

func (d *VDBSBackupSchedulerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_backup_scheduler"
}

func (d *VDBSBackupSchedulerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:    true,
				Description: "Backup scheduler ID",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Scheduler name",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Scheduler status (ACTIVE, INACTIVE, etc)",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Scheduler description",
			},
			"scheduler_type": schema.StringAttribute{
				Computed:    true,
				Description: "Scheduler type (daily, weekly, monthly, etc)",
			},
			"location": schema.StringAttribute{
				Computed:    true,
				Description: "Backup location (block, object)",
			},
			"max_record": schema.Int64Attribute{
				Computed:    true,
				Description: "Maximum number of backup records to keep",
			},
			"current_record": schema.Int64Attribute{
				Computed:    true,
				Description: "Current number of backup records",
			},
			"number_of_instance": schema.Int64Attribute{
				Computed:    true,
				Description: "Number of instances in this scheduler",
			},
			"vpc_id": schema.Int64Attribute{
				Computed:    true,
				Description: "VPC ID",
			},
			"fixed_rate": schema.Int64Attribute{
				Computed:    true,
				Description: "Fixed rate",
			},
			"prev_scheduler_run_time": schema.StringAttribute{
				Computed:    true,
				Description: "Previous scheduler run time",
			},
			"prev_scheduler_run_status": schema.StringAttribute{
				Computed:    true,
				Description: "Previous scheduler run status",
			},
			"next_scheduler_run_time": schema.StringAttribute{
				Computed:    true,
				Description: "Next scheduler run time (ISO 8601)",
			},
			"create_time": schema.StringAttribute{
				Computed:    true,
				Description: "Creation time (ISO 8601)",
			},
			"update_time": schema.StringAttribute{
				Computed:    true,
				Description: "Last update time (ISO 8601)",
			},
		},
	}
}

func (d *VDBSBackupSchedulerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
}

func (d *VDBSBackupSchedulerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg VDBSBackupSchedulerDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Call API to list backup schedulers
	body := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   100,
		"filters":    []interface{}{},
		"selected":   6,
		"hostId":     6,
		"customerId": d.customerID,
		"planType":   "dbs",
	}

	apiResp, diags := callAPI(ctx, d.client, pathBackupSchedulerList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"DBS backup scheduler not found",
			fmt.Sprintf("DBS backup scheduler not found with id %d", cfg.ID.ValueInt64()),
		)
		return
	}

	if apiResp == nil || apiResp.Data == nil {
		resp.Diagnostics.AddError(
			"DBS backup scheduler not found",
			fmt.Sprintf("DBS backup scheduler not found with id %d", cfg.ID.ValueInt64()),
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
	var schedulers []map[string]interface{}
	if itemsRaw, ok := listData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					schedulers = append(schedulers, itemMap)
				}
			}
		}
	}

	// Find scheduler by ID
	schedulerID := cfg.ID.ValueInt64()
	var foundScheduler map[string]interface{}
	for _, scheduler := range schedulers {
		if asInt64(scheduler, "id") == schedulerID {
			foundScheduler = scheduler
			break
		}
	}

	if foundScheduler == nil {
		resp.Diagnostics.AddError(
			"DBS backup scheduler not found",
			fmt.Sprintf("No backup scheduler found with id %d", schedulerID),
		)
		return
	}

	// Map response to model
	cfg.ID = types.Int64Value(asInt64(foundScheduler, "id"))
	cfg.Name = types.StringValue(asString(foundScheduler, "name"))
	cfg.Status = types.StringValue(asString(foundScheduler, "status"))
	cfg.Description = types.StringValue(asString(foundScheduler, "description"))
	cfg.SchedulerType = types.StringValue(asString(foundScheduler, "schedulerType"))
	cfg.Location = types.StringValue(asString(foundScheduler, "location"))
	cfg.MaxRecord = types.Int64Value(asInt64(foundScheduler, "maxRecord"))
	cfg.CurrentRecord = types.Int64Value(asInt64(foundScheduler, "currentRecord"))
	cfg.NumberOfInstance = types.Int64Value(asInt64(foundScheduler, "numberOfInstance"))
	cfg.VpcID = types.Int64Value(asInt64(foundScheduler, "vpcId"))
	cfg.FixedRate = types.Int64Value(asInt64(foundScheduler, "fixedRate"))
	cfg.PrevSchedulerRunStatus = types.StringValue(asString(foundScheduler, "prevSchedulerRunStatus"))

	// Format timestamps
	if prevRunTime, ok := foundScheduler["prevSchedulerRunTime"].(string); ok && prevRunTime != "" {
		cfg.PrevSchedulerRunTime = types.StringValue(prevRunTime)
	}

	if nextRunTime, ok := foundScheduler["nextSchedulerRunTime"].(string); ok && nextRunTime != "" {
		cfg.NextSchedulerRunTime = types.StringValue(nextRunTime)
	}

	if createTime, ok := foundScheduler["createTime"].(string); ok && createTime != "" {
		cfg.CreateTime = types.StringValue(createTime)
	}

	if updateTime, ok := foundScheduler["updateTime"].(string); ok && updateTime != "" {
		cfg.UpdateTime = types.StringValue(updateTime)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
