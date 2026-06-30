// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

var (
	_ resource.Resource                = &schedulerResource{}
	_ resource.ResourceWithConfigure   = &schedulerResource{}
	_ resource.ResourceWithImportState = &schedulerResource{}
)

func NewSchedulerResource() resource.Resource { // matching registration NewSchedulerResource in provider.go
	return &schedulerResource{}
}

type schedulerResource struct {
	clientData *providerdata.ProviderData
}

type SchedulerResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	StartTime  types.String `tfsdk:"start_time"`
	FinishTime types.String `tfsdk:"finish_time"`
	Cycle      types.Int64  `tfsdk:"cycle"`
	Unit       types.String `tfsdk:"unit"`
	Quantity   types.Int64  `tfsdk:"quantity"`
	HostID     types.Int64  `tfsdk:"host_id"`
	PlanType   types.String `tfsdk:"plan_type"`
	Status     types.String `tfsdk:"status"`
	ClusterID  types.Int64  `tfsdk:"cluster_id"`
	VolumeIDs  types.List   `tfsdk:"volume_ids"`
}

func (r *schedulerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_scheduler"
}

func (r *schedulerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *schedulerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a VKS Scheduler resource to manage block storage backup schedules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the backup scheduler.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the backup schedule.",
				Required:            true,
			},
			"start_time": schema.StringAttribute{
				MarkdownDescription: "Start time of the schedule, formatted exactly as `YYYY-MM-DD HH:mm:ss`. Must be in the future.",
				Required:            true,
			},
			"finish_time": schema.StringAttribute{
				MarkdownDescription: "Finish time of the schedule, formatted exactly as `YYYY-MM-DD HH:mm:ss`. Must be after `start_time`.",
				Required:            true,
			},
			"cycle": schema.Int64Attribute{
				MarkdownDescription: "Cycle interval in seconds (e.g. 86400 for 1 day). Must correspond to the `unit`.",
				Required:            true,
			},
			"unit": schema.StringAttribute{
				MarkdownDescription: "Unit of interval. Supported values: `day`, `hour`, `week`.",
				Required:            true,
			},
			"quantity": schema.Int64Attribute{
				MarkdownDescription: "Number of backup records to retain before older ones are rotated out.",
				Required:            true,
			},
			"host_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the host. If not set, inherits from provider config.",
				Optional:            true,
				Computed:            true,
			},
			"plan_type": schema.StringAttribute{
				MarkdownDescription: "Plan type, default is `k8s`. Leave it empty to use default.",
				Optional:            true,
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current status of the backup schedule (e.g., `ACTIVE`).",
				Computed:            true,
			},
			"cluster_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the K8s cluster where the block storage volumes belong.",
				Required:            true,
			},
			"volume_ids": schema.ListAttribute{
				MarkdownDescription: "List of block storage volume IDs to backup. These volumes must be attached to the cluster.",
				Required:            true,
				ElementType:         types.Int64Type,
			},
		},
	}
}

func (r *schedulerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !plan.PlanType.IsNull() && !plan.PlanType.IsUnknown() {
		planType = plan.PlanType.ValueString()
	}

	var volumeIDs []int64
	resp.Diagnostics.Append(plan.VolumeIDs.ElementsAs(ctx, &volumeIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	blockStorageIds := make([]interface{}, len(volumeIDs))
	for i, vid := range volumeIDs {
		blockStorageIds[i] = vid
	}

	hostID := r.clientData.HostID
	if !plan.HostID.IsNull() && !plan.HostID.IsUnknown() {
		hostID = plan.HostID.ValueInt64()
	}
	if hostID == 0 {
		resp.Diagnostics.AddError("Missing Host ID", "host_id must be configured in either the resource or the provider block.")
		return
	}

	payload := map[string]interface{}{
		"id":               nil,
		"name":             plan.Name.ValueString(),
		"start_time":       plan.StartTime.ValueString(),
		"finish_time":      plan.FinishTime.ValueString(),
		"cycle":            plan.Cycle.ValueInt64(),
		"unit":             plan.Unit.ValueString(),
		"quantity":         plan.Quantity.ValueInt64(),
		"block_storage_id": blockStorageIds,
		"type":             "full",
		"host_id":          hostID,
		"customer_id":      r.clientData.CustomerID,
		"planType":         planType,
		"cluster_id":       plan.ClusterID.ValueInt64(),
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerCreateEdit, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var createdID string
	if err := json.Unmarshal(apiResp.Data, &createdID); err != nil {
		var createdInt int
		if err2 := json.Unmarshal(apiResp.Data, &createdInt); err2 == nil {
			createdID = strconv.Itoa(createdInt)
		} else {
			createdID = strings.Trim(string(apiResp.Data), ` "`)
		}
	}

	plan.ID = types.StringValue(createdID)
	plan.PlanType = types.StringValue(planType)
	plan.Status = types.StringValue("success")
	plan.HostID = types.Int64Value(hostID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *schedulerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchedulerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !state.PlanType.IsNull() && !state.PlanType.IsUnknown() {
		planType = state.PlanType.ValueString()
	}

	payload := map[string]interface{}{
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
		"size":        0,
		"sorts":       []interface{}{},
		"host_id":     state.HostID.ValueInt64(),
		"customer_id": r.clientData.CustomerID,
		"planType":    planType,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerListPaging, payload)
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
		resp.State.RemoveResource(ctx)
		return
	}

	itemsList, ok := itemsVal.([]interface{})
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	targetID := state.ID.ValueString()
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
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(asString(foundItem, "name"))
	state.Status = types.StringValue(asString(foundItem, "status"))

	startDateRaw := asString(foundItem, "startDate")
	if strings.Contains(startDateRaw, ".") {
		startDateRaw = strings.Split(startDateRaw, ".")[0]
	}
	state.StartTime = types.StringValue(startDateRaw)

	finishAtRaw := asString(foundItem, "finishAt")
	if strings.Contains(finishAtRaw, ".") {
		finishAtRaw = strings.Split(finishAtRaw, ".")[0]
	}
	state.FinishTime = types.StringValue(finishAtRaw)

	state.Quantity = types.Int64Value(asInt64(foundItem, "quantityCycle"))
	state.Unit = types.StringValue(asString(foundItem, "unitCycle"))

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
	state.Cycle = types.Int64Value(cycleSec)

	// Parse blockStorageId list from response
	var volumeIDs []int64
	if bsVal, ok := foundItem["blockStorageId"]; ok && bsVal != nil {
		if bsList, ok := bsVal.([]interface{}); ok {
			for _, v := range bsList {
				switch val := v.(type) {
				case float64:
					volumeIDs = append(volumeIDs, int64(val))
				case int:
					volumeIDs = append(volumeIDs, int64(val))
				case int64:
					volumeIDs = append(volumeIDs, val)
				case string:
					if n, err := strconv.ParseInt(val, 10, 64); err == nil {
						volumeIDs = append(volumeIDs, n)
					}
				}
			}
		}
	}
	volList, d := types.ListValueFrom(ctx, types.Int64Type, volumeIDs)
	resp.Diagnostics.Append(d...)
	state.VolumeIDs = volList

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *schedulerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !plan.PlanType.IsNull() && !plan.PlanType.IsUnknown() {
		planType = plan.PlanType.ValueString()
	}

	idVal, err := strconv.Atoi(plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Update Error", "Invalid scheduler ID format")
		return
	}

	var volumeIDs []int64
	resp.Diagnostics.Append(plan.VolumeIDs.ElementsAs(ctx, &volumeIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	blockStorageIds := make([]interface{}, len(volumeIDs))
	for i, vid := range volumeIDs {
		blockStorageIds[i] = vid
	}

	hostID := r.clientData.HostID
	if !plan.HostID.IsNull() && !plan.HostID.IsUnknown() {
		hostID = plan.HostID.ValueInt64()
	}
	if hostID == 0 {
		resp.Diagnostics.AddError("Missing Host ID", "host_id must be configured in either the resource or the provider block.")
		return
	}

	payload := map[string]interface{}{
		"id":               idVal,
		"name":             plan.Name.ValueString(),
		"start_time":       plan.StartTime.ValueString(),
		"finish_time":      plan.FinishTime.ValueString(),
		"cycle":            plan.Cycle.ValueInt64(),
		"unit":             plan.Unit.ValueString(),
		"quantity":         plan.Quantity.ValueInt64(),
		"block_storage_id": blockStorageIds,
		"type":             "full",
		"host_id":          hostID,
		"customer_id":      r.clientData.CustomerID,
		"planType":         planType,
		"cluster_id":       plan.ClusterID.ValueInt64(),
	}

	_, diags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerCreateEdit, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.PlanType = types.StringValue(planType)
	plan.Status = types.StringValue("success")
	plan.HostID = types.Int64Value(hostID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *schedulerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchedulerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !state.PlanType.IsNull() && !state.PlanType.IsUnknown() {
		planType = state.PlanType.ValueString()
	}

	idVal, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Delete Error", "Invalid scheduler ID format")
		return
	}

	payload := map[string]interface{}{
		"id":          idVal,
		"host_id":     state.HostID.ValueInt64(),
		"customer_id": r.clientData.CustomerID,
		"planType":    planType,
	}

	_, diags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerDelete, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *schedulerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Import scheduler expects format: <cluster_id>/<host_id>/<schedule_id>")
		return
	}

	clusterID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Cluster ID", "Cluster ID must be an integer")
		return
	}
	hostID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Host ID", "Host ID must be an integer")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_id"), hostID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}
