package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ resource.Resource                = (*BackupPlanResource)(nil)
	_ resource.ResourceWithConfigure   = (*BackupPlanResource)(nil)
	_ resource.ResourceWithImportState = (*BackupPlanResource)(nil)
)

type BackupPlanResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupPlanResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	BackupCycleID     types.Int64  `tfsdk:"backup_cycle_id"`
	StartDayBackup    types.String `tfsdk:"start_day_backup"`
	TimeBackup        types.String `tfsdk:"time_backup"`
	NumberOfRecord    types.Int64  `tfsdk:"number_of_record"`
	VolumeIDs         types.List   `tfsdk:"volume_ids"`
	VpcID             types.String `tfsdk:"vpc_id"`
	Status            types.String `tfsdk:"status"`
	BackupCycleName   types.String `tfsdk:"backup_cycle_name"`
}

func NewBackupPlanResource() resource.Resource { return &BackupPlanResource{} }

func (r *BackupPlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_plan"
}

func (r *BackupPlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Backup Plan for scheduling volume backups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Backup Plan ID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable Backup Plan name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Description of the Backup Plan.",
			},
			"backup_cycle_id": schema.Int64Attribute{
				Required:    true,
				Description: "ID of the backup cycle (e.g., daily, weekly).",
			},
			"start_day_backup": schema.StringAttribute{
				Required:    true,
				Description: "Start date for backup in YYYY-MM-DD format.",
			},
			"time_backup": schema.StringAttribute{
				Required:    true,
				Description: "Time for backup in HH:MM:SS format.",
			},
			"number_of_record": schema.Int64Attribute{
				Required:    true,
				Description: "Number of backup records to retain.",
			},
			"volume_ids": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "List of volume IDs to include in the backup plan.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the Backup Plan.",
			},
			"backup_cycle_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the backup cycle.",
			},
		},
	}
}

func (r *BackupPlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *BackupPlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BackupPlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }

	vpcID := defaultIfEmpty(plan.VpcID, r.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("vpc_id"), "Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}

	// Convert volume IDs to the expected format
	var volumeIDs []string
	resp.Diagnostics.Append(plan.VolumeIDs.ElementsAs(ctx, &volumeIDs, false)...)
	if resp.Diagnostics.HasError() { return }

	listVolumes := make([]map[string]interface{}, len(volumeIDs))
	for i, vid := range volumeIDs {
		listVolumes[i] = map[string]interface{}{"id": parseInt(vid)}
	}

	body := map[string]interface{}{
		"vpc_id":           vpcID,
		"customer_id":      r.customerID,
		"name":             plan.Name.ValueString(),
		"description":      plan.Description.ValueString(),
		"vttBackupCycleId": plan.BackupCycleID.ValueInt64(),
		"startDayBackup":   plan.StartDayBackup.ValueString(),
		"timeBackup":       plan.TimeBackup.ValueString(),
		"numberOfRecord":   plan.NumberOfRecord.ValueInt64(),
		"listVolumes":    listVolumes,
	}

	apiResp, diags := callAPI(ctx, r.client, pathBackupPlanCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    int64  `json:"data"`
	}
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		resp.Diagnostics.AddError("Parse Error", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", result.Data))
	plan.VpcID = types.StringValue(vpcID)

	// Fetch details to get computed fields
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() { return }

	// Report creation status to the user so they are aware of async state.
	switch strings.ToUpper(plan.Status.ValueString()) {
	case "ERROR", "FAILED":
		resp.Diagnostics.AddError(
			"Backup Plan entered an error state",
			fmt.Sprintf("Backup Plan %s has status %q. Check the ViettelIDC console.", plan.ID.ValueString(), plan.Status.ValueString()),
		)
		return
	case "ACTIVE", "SUCCESS", "":
		// ready — no action needed
	default:
		resp.Diagnostics.AddWarning(
			"Backup Plan is still provisioning",
			fmt.Sprintf("Backup Plan %s has status %q. Run 'terraform refresh' to update once it is ready.", plan.ID.ValueString(), plan.Status.ValueString()),
		)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *BackupPlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BackupPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }

	r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() { return }

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *BackupPlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BackupPlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }

	var state BackupPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }

	body := map[string]interface{}{
		"vpc_id":           plan.VpcID.ValueString(),
		"customer_id":      r.customerID,
		"id":               plan.ID.ValueString(),
		"name":             plan.Name.ValueString(),
		"description":      plan.Description.ValueString(),
		"vttBackupCycleId": plan.BackupCycleID.ValueInt64(),
		"startDayBackup":   plan.StartDayBackup.ValueString(),
		"timeBackup":       plan.TimeBackup.ValueString(),
		"numberOfRecord":   plan.NumberOfRecord.ValueInt64(),
	}

	_, diags := callAPI(ctx, r.client, pathBackupPlanUpdate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() { return }

	// Handle volume changes separately if needed
	// For now, we'll refetch and update the state
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() { return }

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *BackupPlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BackupPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }

	body := map[string]interface{}{
		"vpc_id":       state.VpcID.ValueString(),
		"customer_id":  r.customerID,
		"id":           state.ID.ValueString(),
		"isAutoDelete": false,
	}

	apiResp, diags := callAPI(ctx, r.client, pathBackupPlanDelete, body)
	resp.Diagnostics.Append(diags...)

	if apiResp != nil && !apiResp.IsSuccess() {
		if isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.AddError("Delete Error", apiResp.Message)
	}
}

func (r *BackupPlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *BackupPlanResource) readAndMerge(ctx context.Context, model *BackupPlanResourceModel, diags *diag.Diagnostics) {
	if model.VpcID.ValueString() == "" || model.ID.ValueString() == "" {
		return
	}

	body := map[string]interface{}{
		"vpc_id":      model.VpcID.ValueString(),
		"customer_id": r.customerID,
		"page_index":  0,
		"page_size":   1000,
		"filters":     []map[string]interface{}{},
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathBackupPlanList, body)
	diags.Append(callDiags...)
	if diags.HasError() { return }

	var listResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Content []struct {
				ID               int64  `json:"id"`
				Name             string `json:"name"`
				Description      string `json:"description"`
				VttBackupCycleID int64  `json:"vttBackupCycleId"`
				BackupCycleName  string `json:"backupCycleName"`
				StartDayBackup   string `json:"startDayBackup"`
				TimeBackup       string `json:"timeBackup"`
				NumberOfRecord   int    `json:"numberOfRecord"`
				Status           string `json:"status"`
				ListVolume       []struct {
					ID int64 `json:"id"`
				} `json:"listVolume"`
			} `json:"content"`
		} `json:"data"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	targetID := model.ID.ValueString()
	for _, item := range listResp.Data.Content {
		if fmt.Sprintf("%d", item.ID) == targetID {
			model.Name = types.StringValue(item.Name)
			model.Description = types.StringValue(item.Description)
			model.BackupCycleID = types.Int64Value(item.VttBackupCycleID)
			model.BackupCycleName = types.StringValue(item.BackupCycleName)
			model.StartDayBackup = types.StringValue(item.StartDayBackup)
			model.TimeBackup = types.StringValue(item.TimeBackup)
			model.NumberOfRecord = types.Int64Value(int64(item.NumberOfRecord))
			model.Status = types.StringValue(item.Status)

			// Fail fast if backup plan entered a terminal error state.
			if st := strings.ToUpper(item.Status); st == "ERROR" || st == "FAILED" {
				diags.AddError(
					"Backup Plan is in error state",
					fmt.Sprintf("Backup Plan %s has status=%s. Destroy and re-create it before proceeding.", targetID, item.Status),
				)
				return
			}

			// Convert volume IDs
			volumeIDs := make([]string, len(item.ListVolume))
			for i, v := range item.ListVolume {
				volumeIDs[i] = fmt.Sprintf("%d", v.ID)
			}
			volList, d := types.ListValueFrom(ctx, types.StringType, volumeIDs)
			diags.Append(d...)
			model.VolumeIDs = volList
			return
		}
	}

	diags.AddError("Not Found", fmt.Sprintf("Backup Plan %s not found", targetID))
}
