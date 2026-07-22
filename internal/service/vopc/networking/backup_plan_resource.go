// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*BackupPlanResource)(nil)
	_ resource.ResourceWithConfigure   = (*BackupPlanResource)(nil)
	_ resource.ResourceWithImportState = (*BackupPlanResource)(nil)
)

// defaultBackupBaseURL is the public gateway that serves the backup service.
// Override with VIETTELIDC_BACKUP_BASE_URL.
const defaultBackupBaseURL = "https://api.viettelidc.com.vn"

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// BackupPlanResource implements `viettelidc_ovpc_backup_plan`.
//
// It talks to the backup service on the public gateway:
//
//	POST   /backup/api/v1/vpc/{vpcID}/backup-schedules
//	GET    /backup/api/v1/vpc/{vpcID}/backup-schedules/{id}
//	DELETE /backup/api/v1/vpc/{vpcID}/backup-schedules/{id}
//
// The /csa/api/v1/storage/backup-plan/* endpoints this resource used to call
// are a different, empty store that rejects every create with
// ERROR_NOT_ALLOWED_CREATE_BACKUP_SCHEDULER_ACTION.
type BackupPlanResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupPlanResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Cycle          types.String `tfsdk:"cycle"`
	StartDate      types.String `tfsdk:"start_date"`
	StartTime      types.String `tfsdk:"start_time"`
	NumberOfRecord types.Int64  `tfsdk:"number_of_record"`
	VMIDs          types.List   `tfsdk:"vm_ids"`
	Status         types.String `tfsdk:"status"`
	NextTime       types.String `tfsdk:"next_time"`
	VpcID          types.String `tfsdk:"vpc_id"`
}

func NewBackupPlanResource() resource.Resource { return &BackupPlanResource{} }

func (r *BackupPlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_plan"
}

func (r *BackupPlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	forceNew := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Backup Schedule — periodic backups of whole VMs. " +
			"The API exposes create, read and delete only, so every configurable attribute forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Backup schedule UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Backup schedule name.",
				PlanModifiers: forceNew,
			},
			"description": schema.StringAttribute{
				Optional:      true,
				Description:   "Optional description.",
				PlanModifiers: forceNew,
			},
			"cycle": schema.StringAttribute{
				Required:      true,
				Description:   `Backup frequency, e.g. "DAILY".`,
				PlanModifiers: forceNew,
			},
			"start_date": schema.StringAttribute{
				Required:      true,
				Description:   "First backup date, YYYY-MM-DD.",
				PlanModifiers: forceNew,
			},
			"start_time": schema.StringAttribute{
				Required:      true,
				Description:   "Time of day to run the backup, HH:MM:SS.",
				PlanModifiers: forceNew,
			},
			"number_of_record": schema.Int64Attribute{
				Required:    true,
				Description: "How many backup records to keep.",
			},
			"vm_ids": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "VMs to back up. Accepts either the numeric instance id " +
					"(viettelidc_ovpc_instance.x.id) or the backup service's VM UUID — numeric " +
					"ids are resolved automatically. Only VMs the backup service offers for the " +
					"VPC can be used.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current schedule status.",
			},
			"next_time": schema.StringAttribute{
				Computed:    true,
				Description: "Timestamp of the next scheduled run.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "VPC ID. Falls back to the provider default vpc_id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *BackupPlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

// backupClient returns the shared client aimed at the backup gateway.
func (r *BackupPlanResource) backupClient() *client.Client {
	baseURL := os.Getenv("VIETTELIDC_BACKUP_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBackupBaseURL
	}
	return r.client.WithBaseURL(baseURL)
}

func schedulesPath(vpcID string) string {
	return fmt.Sprintf("/backup/api/v1/vpc/%s/backup-schedules", vpcID)
}

func (r *BackupPlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BackupPlanResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	vmIDs, d := r.resolveVMIDs(ctx, plan.VMIDs, vpcID)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"name":           plan.Name.ValueString(),
		"cycle":          plan.Cycle.ValueString(),
		"numberOfRecord": fmt.Sprintf("%d", plan.NumberOfRecord.ValueInt64()),
		"vmIds":          vmIDs,
		"startTime":      plan.StartTime.ValueString(),
		"startDate":      plan.StartDate.ValueString(),
		"vpcId":          parseInt(vpcID),
		"customerId":     parseInt(r.customerID),
	}
	if v := plan.Description.ValueString(); v != "" {
		body["description"] = v
	}

	raw, err := r.backupClient().DoMethod(ctx, http.MethodPost, schedulesPath(vpcID), body)
	if err != nil {
		resp.Diagnostics.AddError("Backup schedule create failed", err.Error())
		return
	}
	id, err := extractBackupScheduleID(raw)
	if err != nil {
		resp.Diagnostics.AddError("Backup schedule create response has no id", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)
	if !r.readInto(ctx, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Backup schedule vanished", "the schedule was created but could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BackupPlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BackupPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.readInto(ctx, &state, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every configurable attribute forces replacement.
func (r *BackupPlanResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *BackupPlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BackupPlanResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	p := schedulesPath(vpcID) + "/" + state.ID.ValueString()
	if _, err := r.backupClient().DoMethod(ctx, http.MethodDelete, p, nil); err != nil {
		if isBackupNotFound(err) {
			return // already gone — idempotent
		}
		resp.Diagnostics.AddError("Backup schedule delete failed", err.Error())
	}
}

func (r *BackupPlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto refreshes m from the API. Returns false when the schedule is gone.
func (r *BackupPlanResource) readInto(ctx context.Context, m *BackupPlanResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	p := schedulesPath(vpcID) + "/" + m.ID.ValueString()
	raw, err := r.backupClient().DoMethod(ctx, http.MethodGet, p, nil)
	if err != nil {
		if isBackupNotFound(err) {
			return false
		}
		diags.AddError("Backup schedule read failed", err.Error())
		return true
	}

	var env struct {
		Data struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Description    string `json:"description"`
			Cycle          string `json:"cycle"`
			StartTime      string `json:"startTime"`
			StartDate      string `json:"startDate"`
			NumberOfRecord int64  `json:"numberOfRecord"`
			Status         string `json:"status"`
			NextTime       string `json:"nextTime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		diags.AddError("Backup schedule decode failed", err.Error())
		return true
	}
	d := env.Data
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.Cycle = types.StringValue(d.Cycle)
	m.StartTime = types.StringValue(d.StartTime)
	m.StartDate = types.StringValue(d.StartDate)
	m.NumberOfRecord = types.Int64Value(d.NumberOfRecord)
	m.Status = types.StringValue(d.Status)
	m.NextTime = types.StringValue(d.NextTime)
	m.VpcID = types.StringValue(vpcID)
	if d.Description != "" {
		m.Description = types.StringValue(d.Description)
	}
	return true
}

// resolveVMIDs turns the configured vm_ids into the UUIDs the backup service
// expects. UUIDs pass through; a numeric IaC instance id is resolved by looking
// up the instance name and matching it against the VMs the backup service
// offers for this VPC.
func (r *BackupPlanResource) resolveVMIDs(ctx context.Context, list types.List, vpcID string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var configured []string
	diags.Append(list.ElementsAs(ctx, &configured, false)...)
	if diags.HasError() {
		return nil, diags
	}

	var backupVMs map[string]string // name -> uuid, fetched lazily
	out := make([]string, 0, len(configured))
	for _, v := range configured {
		if uuidRe.MatchString(v) {
			out = append(out, v)
			continue
		}
		if backupVMs == nil {
			var err error
			backupVMs, err = r.listBackupVMs(ctx, vpcID)
			if err != nil {
				diags.AddError("Cannot list VMs available for backup", err.Error())
				return nil, diags
			}
		}
		name, err := r.instanceName(ctx, v, vpcID)
		if err != nil {
			diags.AddError("Cannot resolve instance "+v, err.Error())
			return nil, diags
		}
		uuid, ok := backupVMs[name]
		if !ok {
			names := make([]string, 0, len(backupVMs))
			for n := range backupVMs {
				names = append(names, n)
			}
			diags.AddError(
				"VM is not available for backup",
				fmt.Sprintf("instance %s (%q) is not offered by the backup service for VPC %s. Available: %s",
					v, name, vpcID, strings.Join(names, ", ")),
			)
			return nil, diags
		}
		out = append(out, uuid)
	}
	return out, diags
}

// listBackupVMs returns name -> UUID for the VMs the backup service accepts.
func (r *BackupPlanResource) listBackupVMs(ctx context.Context, vpcID string) (map[string]string, error) {
	raw, err := r.backupClient().DoMethod(ctx, http.MethodGet,
		fmt.Sprintf("/backup/api/v1/vpc/%s/vms", vpcID), nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(env.Items))
	for _, it := range env.Items {
		out[it.Name] = it.ID
	}
	return out, nil
}

// instanceName looks up an instance's name from its numeric id via the IaC API.
func (r *BackupPlanResource) instanceName(ctx context.Context, instanceID, vpcID string) (string, error) {
	apiResp, diags := callAPI(ctx, r.client, pathVMDetail, map[string]interface{}{
		"instance_id": instanceID,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	})
	if diags.HasError() {
		msg := "unknown error"
		if errs := diags.Errors(); len(errs) > 0 {
			msg = errs[0].Detail()
		}
		return "", fmt.Errorf("vm detail: %s", msg)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return "", err
	}
	name := asString(data, "name")
	if name == "" {
		return "", fmt.Errorf("vm %s has no name", instanceID)
	}
	return name, nil
}

// ---------- Pure helpers ----------

func extractBackupScheduleID(raw []byte) (string, error) {
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", err
	}
	if env.Data.ID != "" {
		return env.Data.ID, nil
	}
	if env.ID != "" {
		return env.ID, nil
	}
	return "", fmt.Errorf("no id in response: %.256s", string(raw))
}

func isBackupNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 404") || strings.Contains(s, "ERROR.NOT.FOUND")
}
