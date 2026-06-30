// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/client"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

var (
	_ resource.Resource                = &backupRecordResource{}
	_ resource.ResourceWithConfigure   = &backupRecordResource{}
	_ resource.ResourceWithImportState = &backupRecordResource{}
)

func NewBackupRecordResource() resource.Resource {
	return &backupRecordResource{}
}

type backupRecordResource struct {
	clientData *providerdata.ProviderData
}

type BackupRecordResourceModel struct {
	ID        types.String `tfsdk:"id"`
	VolumeID  types.Int64  `tfsdk:"volume_id"`
	ClusterID types.Int64  `tfsdk:"cluster_id"`
	HostID    types.Int64  `tfsdk:"host_id"`
	PlanType  types.String `tfsdk:"plan_type"`
	Name      types.String `tfsdk:"name"`
	Status    types.String `tfsdk:"status"`
	CreatedAt types.String `tfsdk:"created_at"`
	Size      types.Int64  `tfsdk:"size"`
}

func (r *backupRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_backup_record"
}

func (r *backupRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *backupRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a VKS Backup Record resource to manage manual block storage backups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the manual backup record.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"volume_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the block storage volume to backup.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"cluster_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the K8s cluster.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"host_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the host. If not set, inherits from provider config.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"plan_type": schema.StringAttribute{
				MarkdownDescription: "Plan type, default is `k8s`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the snapshot backup.",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Status of the manual backup record.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation time of the backup record.",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Size of the backup snapshot.",
				Computed:            true,
			},
		},
	}
}

func (r *backupRecordResource) findRecordByName(ctx context.Context, hostID int64, name string) (map[string]interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics
	payload := map[string]interface{}{
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
		"size":        0,
		"sorts":       []interface{}{},
		"host_id":     hostID,
		"customer_id": r.clientData.CustomerID,
		"planType":    "k8s",
	}
	apiResp, callDiags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerBackupList, payload)
	diags.Append(callDiags...)
	if diags.HasError() {
		return nil, diags
	}
	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		diags.AddError("Parse Error", "Failed to parse backup list-paging response")
		return nil, diags
	}
	itemsVal, ok := responseMap["items"]
	if !ok || itemsVal == nil {
		return nil, diags
	}
	itemsList, ok := itemsVal.([]interface{})
	if !ok {
		return nil, diags
	}
	for _, itemRaw := range itemsList {
		if itemMap, ok := itemRaw.(map[string]interface{}); ok {
			if asString(itemMap, "name") == name {
				return itemMap, diags
			}
		}
	}
	return nil, diags
}

func (r *backupRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BackupRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planType := "k8s"
	if !plan.PlanType.IsNull() && !plan.PlanType.IsUnknown() {
		planType = plan.PlanType.ValueString()
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
		"id":          plan.VolumeID.ValueInt64(),
		"cluster_id":  plan.ClusterID.ValueInt64(),
		"host_id":     hostID,
		"customer_id": r.clientData.CustomerID,
		"planType":    planType,
	}

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathK8sBackupManualCreate, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var createMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &createMap); err != nil {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse create response: "+err.Error())
		return
	}

	snapshotName := ""
	if valuesVal, ok := createMap["values"]; ok && valuesVal != nil {
		if valuesList, ok := valuesVal.([]interface{}); ok {
			for _, valRaw := range valuesList {
				if valMap, ok := valRaw.(map[string]interface{}); ok {
					if asString(valMap, "name") == "snapshot_name" {
						snapshotName = asString(valMap, "value")
						break
					}
				}
			}
		}
	}
	if snapshotName == "" {
		snapshotName = asString(createMap, "snapshotName")
	}
	if snapshotName == "" {
		snapshotName = asString(createMap, "snapshot_name")
	}
	if snapshotName == "" {
		snapshotName = strings.Trim(string(apiResp.Data), ` "`)
	}

	if snapshotName == "" {
		resp.Diagnostics.AddError("Missing Snapshot Name", "Create backup manual did not return snapshot name")
		return
	}

	var foundRecord map[string]interface{}
	for i := 0; i < 10; i++ {
		var lookupDiags diag.Diagnostics
		foundRecord, lookupDiags = r.findRecordByName(ctx, hostID, snapshotName)
		if lookupDiags.HasError() {
			resp.Diagnostics.Append(lookupDiags...)
			return
		}
		if foundRecord != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if foundRecord == nil {
		resp.Diagnostics.AddError("Not Found", "Failed to find created manual backup record by name: "+snapshotName)
		return
	}

	createdID := asString(foundRecord, "id")
	plan.ID = types.StringValue(createdID)
	plan.PlanType = types.StringValue(planType)
	plan.Name = types.StringValue(asString(foundRecord, "name"))
	plan.Status = types.StringValue(asString(foundRecord, "status"))
	plan.Size = types.Int64Value(asInt64(foundRecord, "size"))

	createdAtRaw := asString(foundRecord, "createdAt")
	if strings.Contains(createdAtRaw, ".") {
		createdAtRaw = strings.Split(createdAtRaw, ".")[0]
	}
	plan.CreatedAt = types.StringValue(createdAtRaw)

	// Poll until AVAILABLE
	for i := 0; i < 45; i++ {
		readPayload := map[string]interface{}{
			"pageIndex":   0,
			"pageSize":    100,
			"filters":     []interface{}{},
			"size":        0,
			"sorts":       []interface{}{},
			"host_id":     hostID,
			"customer_id": r.clientData.CustomerID,
			"planType":    planType,
		}
		readResp, _ := callAPI(ctx, r.clientData.Client, pathK8sSchedulerBackupList, readPayload)
		if readResp != nil && readResp.IsSuccess() {
			var responseMap map[string]interface{}
			if err := json.Unmarshal(readResp.Data, &responseMap); err == nil {
				if itemsVal, ok := responseMap["items"]; ok && itemsVal != nil {
					if itemsList, ok := itemsVal.([]interface{}); ok {
						var updatedRecord map[string]interface{}
						for _, itemRaw := range itemsList {
							if itemMap, ok := itemRaw.(map[string]interface{}); ok {
								if asString(itemMap, "id") == createdID {
									updatedRecord = itemMap
									break
								}
							}
						}
						if updatedRecord != nil {
							status := asString(updatedRecord, "status")
							statusUpper := strings.ToUpper(status)
							if statusUpper == "AVAILABLE" || statusUpper == "SUCCESS" {
								plan.Status = types.StringValue(status)
								plan.Name = types.StringValue(asString(updatedRecord, "name"))
								plan.Size = types.Int64Value(asInt64(updatedRecord, "size"))

								createdAtRaw := asString(updatedRecord, "createdAt")
								if strings.Contains(createdAtRaw, ".") {
									createdAtRaw = strings.Split(createdAtRaw, ".")[0]
								}
								plan.CreatedAt = types.StringValue(createdAtRaw)
								break
							}
							if statusUpper == "FAILED" || statusUpper == "ERROR" {
								resp.Diagnostics.AddError("Creation Error", "Manual backup record reached state: "+status)
								return
							}
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Second)
	}

	if plan.Status.IsNull() || plan.Status.ValueString() == "" {
		resp.Diagnostics.AddWarning("Timeout", "Backup is still creating. It will be updated on next read.")
	}

	plan.HostID = types.Int64Value(hostID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BackupRecordResourceModel
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

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathK8sSchedulerBackupList, payload)
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
	state.Size = types.Int64Value(asInt64(foundItem, "size"))

	createdAtRaw := asString(foundItem, "createdAt")
	if strings.Contains(createdAtRaw, ".") {
		createdAtRaw = strings.Split(createdAtRaw, ".")[0]
	}
	state.CreatedAt = types.StringValue(createdAtRaw)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *backupRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (r *backupRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BackupRecordResourceModel
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
		resp.Diagnostics.AddError("Delete Error", "Invalid backup record ID format")
		return
	}

	payload := map[string]interface{}{
		"id":          idVal,
		"cluster_id":  state.ClusterID.ValueInt64(),
		"host_id":     state.HostID.ValueInt64(),
		"customer_id": r.clientData.CustomerID,
		"planType":    planType,
	}

	var apiResp *client.APIResponse
	var diags diag.Diagnostics
	for i := 0; i < 6; i++ {
		var callDiags diag.Diagnostics
		apiResp, callDiags = callAPI(ctx, r.clientData.Client, pathK8sBackupManualDelete, payload)
		diags = callDiags
		if !callDiags.HasError() {
			break
		}
		if apiResp != nil && strings.Contains(apiResp.Message, "ERROR_VALIDATE_RESOURCE") {
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	for i := 0; i < 30; i++ {
		readPayload := map[string]interface{}{
			"pageIndex":   0,
			"pageSize":    100,
			"filters":     []interface{}{},
			"size":        0,
			"sorts":       []interface{}{},
			"host_id":     state.HostID.ValueInt64(),
			"customer_id": r.clientData.CustomerID,
			"planType":    planType,
		}
		readResp, _ := callAPI(ctx, r.clientData.Client, pathK8sSchedulerBackupList, readPayload)
		if readResp != nil && readResp.IsSuccess() {
			var responseMap map[string]interface{}
			if err := json.Unmarshal(readResp.Data, &responseMap); err == nil {
				if itemsVal, ok := responseMap["items"]; ok && itemsVal != nil {
					if itemsList, ok := itemsVal.([]interface{}); ok {
						found := false
						for _, itemRaw := range itemsList {
							if itemMap, ok := itemRaw.(map[string]interface{}); ok {
								if asString(itemMap, "id") == state.ID.ValueString() {
									status := asString(itemMap, "status")
									statusUpper := strings.ToUpper(status)
									if statusUpper == "DELETED" || statusUpper == "ERROR" {
										break
									}
									found = true
									break
								}
							}
						}
						if !found {
							break
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Second)
	}
}

func (r *backupRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Import backup record expects format: <cluster_id>/<host_id>/<backup_record_id>")
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
