// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*VDBSBackupSchedulerResource)(nil)
	_ resource.ResourceWithConfigure   = (*VDBSBackupSchedulerResource)(nil)
	_ resource.ResourceWithImportState = (*VDBSBackupSchedulerResource)(nil)
)

func NewVDBSBackupSchedulerResource() resource.Resource {
	return &VDBSBackupSchedulerResource{}
}

type VDBSBackupSchedulerResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type VDBSBackupSchedulerResourceModel struct {
	ID             types.Int64  `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	SchedulerType  types.String `tfsdk:"scheduler_type"`
	Location       types.String `tfsdk:"location"`
	MaxRecord      types.Int64  `tfsdk:"max_record"`
	VpcID          types.Int64  `tfsdk:"vpc_id"`
	InstanceID     types.String `tfsdk:"instance_id"`
	Time           types.String `tfsdk:"time"`
	IsDeleteRecord types.Bool   `tfsdk:"is_delete_record"`
	Status         types.String `tfsdk:"status"`
}

func (r *VDBSBackupSchedulerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_backup_scheduler"
}

func (r *VDBSBackupSchedulerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC VDBS Backup Scheduler — configures automated backup schedules for database instances.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "Backup scheduler ID assigned by the system.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Scheduler name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Scheduler description.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scheduler_type": schema.StringAttribute{
				Required:    true,
				Description: "Scheduler type/cycle (e.g. daily, weekly, monthly).",
			},
			"location": schema.StringAttribute{
				Required:    true,
				Description: "Backup location (block, object).",
			},
			"max_record": schema.Int64Attribute{
				Required:    true,
				Description: "Maximum number of backup records to keep (1 to 35).",
			},
			"vpc_id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"instance_id": schema.StringAttribute{
				Required:    true,
				Description: "Database instance ID to bind the backup schedule to.",
			},
			"time": schema.StringAttribute{
				Required:    true,
				Description: "Start time formatted as 'YYYY-MM-DD HH:mm:ss' (in local Vietnam ICT time).",
			},
			"is_delete_record": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Set to true to delete backup records when the scheduler is deleted.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the scheduler (e.g. ACTIVE, INACTIVE).",
			},
		},
	}
}

func (r *VDBSBackupSchedulerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *VDBSBackupSchedulerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VDBSBackupSchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	if vpcID == 0 {
		if r.defaultVpcID != "" {
			if n, err := strconv.ParseInt(r.defaultVpcID, 10, 64); err == nil {
				vpcID = n
			}
		}
	}
	if vpcID == 0 {
		resp.Diagnostics.AddError("vpc_id Required", "vpc_id must be set in the resource block or in the provider configuration.")
		return
	}

	// ICT Timezone conversion logic (GMT+7)
	ict := time.FixedZone("ICT", 7*3600)
	t, err := time.ParseInLocation("2006-01-02 15:04:05", plan.Time.ValueString(), ict)
	if err != nil {
		resp.Diagnostics.AddError("Invalid time format", fmt.Sprintf("Failed to parse time %q: %s. Expected format: YYYY-MM-DD HH:mm:ss", plan.Time.ValueString(), err))
		return
	}
	utcTime := t.UTC()
	dayBackup := utcTime.Format("2006-01-02T15:04:05.000Z")
	timeBackup := utcTime.Format("2006-01-02T15:04:05.000Z")

	instanceUUID, err := r.resolveInstanceUUID(ctx, plan.InstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve database instance UUID", err.Error())
		return
	}

	body := map[string]interface{}{
		"vpcId":         vpcID,
		"name":          plan.Name.ValueString(),
		"location":      plan.Location.ValueString(),
		"instanceIds":   []string{instanceUUID},
		"timeBackup":    timeBackup,
		"time":          plan.Time.ValueString(),
		"schedulerType": plan.SchedulerType.ValueString(),
		"dayBackup":     dayBackup,
		"maxRecord":     plan.MaxRecord.ValueInt64(),
		"instanceId":    instanceUUID,
		"backupMethod":  plan.Location.ValueString(),
		"hostId":        6, // default hostId
		"customerId":    r.customerID,
		"planType":      "dbs",
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body["description"] = plan.Description.ValueString()
	}

	_, callDiags := callAPI(ctx, r.client, pathBackupSchedulerCreate, body)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Poll until the created scheduler appears in the list
	var foundID int64
	for i := 0; i < 36; i++ { // 3 minutes timeout (5s * 36)
		listBody := map[string]interface{}{
			"pageIndex":  0,
			"pageSize":   100,
			"filters":    []interface{}{},
			"hostId":     6,
			"customerId": r.customerID,
			"planType":   "dbs",
		}

		apiResp, _ := callAPI(ctx, r.client, pathBackupSchedulerList, listBody)
		if apiResp != nil && apiResp.IsSuccess() {
			var listData map[string]interface{}
			if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
				if itemsRaw, ok := listData["items"]; ok {
					if itemsArr, ok := itemsRaw.([]interface{}); ok {
						for _, item := range itemsArr {
							if s, ok := item.(map[string]interface{}); ok {
								if asString(s, "name") == plan.Name.ValueString() && asInt64(s, "vpcId") == vpcID {
									foundID = asInt64(s, "id")
									break
								}
							}
						}
					}
				}
			}
		}

		if foundID != 0 {
			break
		}
		time.Sleep(5 * time.Second)
	}

	if foundID == 0 {
		resp.Diagnostics.AddError("Created Scheduler Not Found", "Failed to locate newly created scheduler in listing after timeout.")
		return
	}

	plan.ID = types.Int64Value(foundID)
	plan.VpcID = types.Int64Value(vpcID)

	// Fetch detail to update attributes
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSBackupSchedulerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VDBSBackupSchedulerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found := r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VDBSBackupSchedulerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VDBSBackupSchedulerResourceModel
	var state VDBSBackupSchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	if vpcID == 0 {
		vpcID = state.VpcID.ValueInt64()
	}

	ict := time.FixedZone("ICT", 7*3600)
	t, err := time.ParseInLocation("2006-01-02 15:04:05", plan.Time.ValueString(), ict)
	if err != nil {
		resp.Diagnostics.AddError("Invalid time format", fmt.Sprintf("Failed to parse time %q: %s. Expected format: YYYY-MM-DD HH:mm:ss", plan.Time.ValueString(), err))
		return
	}
	utcTime := t.UTC()
	dayBackup := utcTime.Format("2006-01-02T15:04:05.000Z")
	timeBackup := utcTime.Format("2006-01-02T15:04:05.000Z")

	instanceUUID, err := r.resolveInstanceUUID(ctx, plan.InstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve database instance UUID", err.Error())
		return
	}

	body := map[string]interface{}{
		"id":            state.ID.ValueInt64(),
		"name":          plan.Name.ValueString(),
		"status":        state.Status.ValueString(),
		"vpcId":         vpcID,
		"schedulerType": plan.SchedulerType.ValueString(),
		"location":      plan.Location.ValueString(),
		"time":          plan.Time.ValueString(),
		"dayBackup":     dayBackup,
		"timeBackup":    timeBackup,
		"maxRecord":     plan.MaxRecord.ValueInt64(),
		"instanceIds":   []string{instanceUUID},
		"hostId":        6,
		"customerId":    r.customerID,
		"planType":      "dbs",
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body["description"] = plan.Description.ValueString()
	}

	_, callDiags := callAPI(ctx, r.client, pathBackupSchedulerEdit, body)
	resp.Diagnostics.Append(callDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = state.ID
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSBackupSchedulerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VDBSBackupSchedulerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	isDeleteRecord := false
	if !state.IsDeleteRecord.IsNull() && !state.IsDeleteRecord.IsUnknown() {
		isDeleteRecord = state.IsDeleteRecord.ValueBool()
	}

	body := map[string]interface{}{
		"id":             state.ID.ValueInt64(),
		"isDeleteRecord": isDeleteRecord,
		"hostId":         6,
		"customerId":     r.customerID,
		"planType":       "dbs",
	}

	_, callDiags := callAPI(ctx, r.client, pathBackupSchedulerDelete, body)
	if callDiags.HasError() {
		// Log error, but proceed if already deleted.
		resp.Diagnostics.Append(callDiags...)
		return
	}
}

func (r *VDBSBackupSchedulerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *VDBSBackupSchedulerResource) readAndMerge(ctx context.Context, model *VDBSBackupSchedulerResourceModel, diags *diag.Diagnostics) bool {
	body := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   100,
		"filters":    []interface{}{},
		"hostId":     6,
		"customerId": r.customerID,
		"planType":   "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathBackupSchedulerList, body)
	if callDiags.HasError() {
		diags.Append(callDiags...)
		return false
	}

	if apiResp == nil || apiResp.Data == nil {
		return false
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		diags.AddError("decode error", err.Error())
		return false
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		diags.AddError("decode error", err.Error())
		return false
	}

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

	targetID := model.ID.ValueInt64()
	var found map[string]interface{}
	for _, s := range schedulers {
		if asInt64(s, "id") == targetID {
			found = s
			break
		}
	}

	if found == nil {
		return false
	}

	model.Name = types.StringValue(asString(found, "name"))
	model.Status = types.StringValue(asString(found, "status"))
	model.Description = types.StringValue(asString(found, "description"))
	model.SchedulerType = types.StringValue(asString(found, "schedulerType"))
	model.Location = types.StringValue(asString(found, "location"))
	model.MaxRecord = types.Int64Value(asInt64(found, "maxRecord"))
	model.VpcID = types.Int64Value(asInt64(found, "vpcId"))

	// Keep instance_id preserved if it's set in model and api returns empty array or null for instanceIds
	// Otherwise parse from response if available.
	if instanceIdsRaw, ok := found["instanceIds"]; ok && instanceIdsRaw != nil {
		if arr, ok := instanceIdsRaw.([]interface{}); ok && len(arr) > 0 {
			model.InstanceID = types.StringValue(fmt.Sprintf("%v", arr[0]))
		}
	}

	return true
}

func (r *VDBSBackupSchedulerResource) fetchInstanceUUID(ctx context.Context, serviceInit int64) (string, error) {
	schemaBody := map[string]interface{}{
		"page":        0,
		"pageSize":    100,
		"serviceInit": serviceInit,
		"hostId":      6,
		"customerId":  r.customerID,
		"planType":    "dbs",
	}

	schemaResp, callDiags := callAPI(ctx, r.client, pathDBSchemaList, schemaBody)
	if callDiags.HasError() {
		return "", fmt.Errorf("failed to fetch schema list: %v", callDiags)
	}

	if schemaResp == nil || schemaResp.Data == nil {
		return "", fmt.Errorf("no schema list data found")
	}

	rawSchemas, err := json.Marshal(schemaResp.Data)
	if err != nil {
		return "", err
	}

	var schemaListData map[string]interface{}
	if err := json.Unmarshal(rawSchemas, &schemaListData); err != nil {
		return "", err
	}

	var schemas []map[string]interface{}
	if itemsRaw, ok := schemaListData["items"]; ok {
		if itemsArr, ok := itemsRaw.([]interface{}); ok {
			for _, item := range itemsArr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					schemas = append(schemas, itemMap)
				}
			}
		}
	}

	if len(schemas) == 0 {
		return "", fmt.Errorf("no schemas found for database instance (serviceInit %d)", serviceInit)
	}

	uuid := asString(schemas[0], "instanceId")
	if uuid == "" {
		return "", fmt.Errorf("instanceId is empty in schema list")
	}

	return uuid, nil
}

func (r *VDBSBackupSchedulerResource) resolveInstanceUUID(ctx context.Context, instanceIDInput string) (string, error) {
	isUUID := len(instanceIDInput) == 36 && strings.Contains(instanceIDInput, "-")
	if isUUID {
		return instanceIDInput, nil
	}

	body := map[string]interface{}{
		"customer_id": r.customerID,
		"plan_type":   "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBInstanceList, body)
	if callDiags.HasError() {
		return "", fmt.Errorf("failed to list database instances: %v", callDiags)
	}

	if apiResp == nil || apiResp.Data == nil {
		return "", fmt.Errorf("no database instances data found")
	}

	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		return "", err
	}

	var listData map[string]interface{}
	if err := json.Unmarshal(raw, &listData); err != nil {
		return "", err
	}

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

	var foundInstance map[string]interface{}
	for _, inst := range instances {
		idStr := asString(inst, "id")
		nameStr := asString(inst, "name")
		vttIDStr := asString(inst, "vttDbaasInstanceId")

		if idStr == instanceIDInput || nameStr == instanceIDInput || vttIDStr == instanceIDInput {
			foundInstance = inst
			break
		}
	}

	if foundInstance == nil {
		return "", fmt.Errorf("database instance %q not found", instanceIDInput)
	}

	serviceInit := asInt64(foundInstance, "serviceInit")
	if serviceInit == 0 {
		return "", fmt.Errorf("database instance %q does not have serviceInit populated", instanceIDInput)
	}

	return r.fetchInstanceUUID(ctx, serviceInit)
}
