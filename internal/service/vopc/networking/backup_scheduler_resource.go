// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Status vocabulary of the backup service, taken from live traffic.
// The schedule sits in CREATING or UPDATING while work is in flight; a VM being
// removed reports REMOVING_VM until it is gone. No failure state has been
// observed, so none is hard-coded.
const (
	scheduleStatusSuccess  = "SUCCESS"
	vmStatusRemoving       = "REMOVING_VM"
	backupSchedulerTimeout = 10 * time.Minute
)

var (
	_ resource.Resource                = (*BackupSchedulerResource)(nil)
	_ resource.ResourceWithConfigure   = (*BackupSchedulerResource)(nil)
	_ resource.ResourceWithImportState = (*BackupSchedulerResource)(nil)
)

// BackupSchedulerResource implements `viettelidc_ovpc_backup_scheduler`:
// VM-level backup schedules.
//
// Unlike the POST-only /csa endpoints this one is REST-shaped — the VPC and the
// schedule id are path segments — and the VM membership is a sub-resource:
// PUT on the schedule does NOT accept vm_ids, they are added and removed
// through .../vms. Kong renames the snake_case body to camelCase.
type BackupSchedulerResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type BackupSchedulerResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	Cycle                  types.String `tfsdk:"cycle"`
	StartDate              types.String `tfsdk:"start_date"`
	StartTime              types.String `tfsdk:"start_time"`
	NumberOfRecord         types.Int64  `tfsdk:"number_of_record"`
	VMIDs                  types.Set    `tfsdk:"vm_ids"`
	DeleteRecordsOnDestroy types.Bool   `tfsdk:"delete_records_on_destroy"`
	Status                 types.String `tfsdk:"status"`
	NextTime               types.String `tfsdk:"next_time"`
	VpcID                  types.String `tfsdk:"vpc_id"`
}

func NewBackupSchedulerResource() resource.Resource { return &BackupSchedulerResource{} }

func (r *BackupSchedulerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_backup_scheduler"
}

func (r *BackupSchedulerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Backup Scheduler — a recurring backup of whole VMs. " +
			"Distinct from `viettelidc_ovpc_backup_plan`, which schedules volume backups.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Backup schedule UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Schedule name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Optional description.",
			},
			"cycle": schema.StringAttribute{
				Required:    true,
				Description: `How often the backup runs, e.g. "DAILY".`,
			},
			"start_date": schema.StringAttribute{
				Required:    true,
				Description: "Date of the first run, YYYY-MM-DD.",
			},
			"start_time": schema.StringAttribute{
				Required:    true,
				Description: "Time of day to run, HH:MM:SS.",
			},
			"number_of_record": schema.Int64Attribute{
				Required:    true,
				Description: "How many backup records to retain.",
			},
			"vm_ids": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "UUIDs of the VMs to back up. Changing the set adds or removes " +
					"members in place; it does not replace the schedule.",
			},
			"delete_records_on_destroy": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Whether destroying the schedule also deletes the backup records it produced.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current schedule status.",
			},
			"next_time": schema.StringAttribute{
				Computed:    true,
				Description: "When the schedule next runs.",
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

func (r *BackupSchedulerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

// ---------- Paths ----------

func backupSchedulesPath(vpcID string) string {
	return fmt.Sprintf("/backup/api/v1/vpc/%s/backup-schedules", vpcID)
}

func backupSchedulePath(vpcID, id string) string {
	return backupSchedulesPath(vpcID) + "/" + id
}

func backupScheduleVMsPath(vpcID, id string) string {
	return backupSchedulePath(vpcID, id) + "/vms"
}

// ---------- CRUD ----------

func (r *BackupSchedulerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BackupSchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var vmIDs []string
	resp.Diagnostics.Append(plan.VMIDs.ElementsAs(ctx, &vmIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"name":  plan.Name.ValueString(),
		"cycle": plan.Cycle.ValueString(),
		// The create endpoint wants number_of_record as a string; update wants a
		// number. Mirror the API rather than fight it.
		"number_of_record": fmt.Sprintf("%d", plan.NumberOfRecord.ValueInt64()),
		"vm_ids":           vmIDs,
		"start_time":       plan.StartTime.ValueString(),
		"start_date":       plan.StartDate.ValueString(),
		"vpc_id":           parseInt(vpcID),
		"customer_id":      parseInt(r.customerID),
	}
	if v := plan.Description.ValueString(); v != "" {
		body["description"] = v
	}

	raw, err := r.client.DoMethod(ctx, http.MethodPost, backupSchedulesPath(vpcID), body)
	if err != nil {
		resp.Diagnostics.AddError("Backup scheduler create failed", err.Error())
		return
	}
	id, err := extractBackupSchedulerID(raw)
	if err != nil {
		resp.Diagnostics.AddError("Backup scheduler create response has no id", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)

	// Create is asynchronous: the schedule comes back as CREATING. Returning here
	// would store a transient status and let dependent resources run too early.
	if err := r.waitForSchedule(ctx, vpcID, id); err != nil {
		resp.Diagnostics.AddError("Backup schedule did not become ready", err.Error())
		return
	}

	if !r.readInto(ctx, &plan, &resp.Diagnostics) {
		resp.Diagnostics.AddError("Backup scheduler vanished", "created, but could not be read back")
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BackupSchedulerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BackupSchedulerResourceModel
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

func (r *BackupSchedulerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state BackupSchedulerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	id := state.ID.ValueString()

	// The schedule's own fields go through PUT. vm_ids is deliberately absent:
	// PUT ignores it, membership lives on the .../vms sub-resource.
	putBody := map[string]interface{}{
		"name":             plan.Name.ValueString(),
		"cycle":            plan.Cycle.ValueString(),
		"start_date":       plan.StartDate.ValueString(),
		"start_time":       plan.StartTime.ValueString(),
		"number_of_record": plan.NumberOfRecord.ValueInt64(), // number here, string on create
		"vpc_id":           parseInt(vpcID),
		"customer_id":      parseInt(r.customerID),
	}
	if v := plan.Description.ValueString(); v != "" {
		putBody["description"] = v
	}
	if _, err := r.client.DoMethod(ctx, http.MethodPut, backupSchedulePath(vpcID, id), putBody); err != nil {
		resp.Diagnostics.AddError("Backup scheduler update failed", err.Error())
		return
	}
	if err := r.waitForSchedule(ctx, vpcID, id); err != nil {
		resp.Diagnostics.AddError("Backup schedule did not settle after update", err.Error())
		return
	}

	var want, have []string
	resp.Diagnostics.Append(plan.VMIDs.ElementsAs(ctx, &want, false)...)
	resp.Diagnostics.Append(state.VMIDs.ElementsAs(ctx, &have, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	added, removed := diffStrings(have, want)

	// Each membership change also drives the schedule through UPDATING, so wait
	// it out before the next call and before reading state back.
	if len(added) > 0 {
		if err := r.changeVMs(ctx, http.MethodPost, vpcID, id, added); err != nil {
			resp.Diagnostics.AddError("Cannot add VMs to backup scheduler", err.Error())
			return
		}
		if err := r.waitForSchedule(ctx, vpcID, id); err != nil {
			resp.Diagnostics.AddError("Backup schedule did not settle after adding VMs", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		if err := r.changeVMs(ctx, http.MethodDelete, vpcID, id, removed); err != nil {
			resp.Diagnostics.AddError("Cannot remove VMs from backup scheduler", err.Error())
			return
		}
		if err := r.waitForSchedule(ctx, vpcID, id); err != nil {
			resp.Diagnostics.AddError("Backup schedule did not settle after removing VMs", err.Error())
			return
		}
	}

	plan.ID = state.ID
	plan.VpcID = types.StringValue(vpcID)
	if !r.readInto(ctx, &plan, &resp.Diagnostics) {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BackupSchedulerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BackupSchedulerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	// delete_records is a query parameter; Kong renames it to deleteRecords.
	p := fmt.Sprintf("%s?delete_records=%t",
		backupSchedulePath(vpcID, state.ID.ValueString()),
		state.DeleteRecordsOnDestroy.ValueBool())
	body := map[string]interface{}{
		"vpc_id":      parseInt(vpcID),
		"customer_id": parseInt(r.customerID),
	}
	if _, err := r.client.DoMethod(ctx, http.MethodDelete, p, body); err != nil {
		if isBackupSchedulerNotFound(err) {
			return // already gone — idempotent
		}
		resp.Diagnostics.AddError("Backup scheduler delete failed", err.Error())
	}
}

func (r *BackupSchedulerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------- Helpers ----------

// changeVMs adds (POST) or removes (DELETE) schedule members.
func (r *BackupSchedulerResource) changeVMs(ctx context.Context, method, vpcID, id string, vmIDs []string) error {
	body := map[string]interface{}{
		"vm_ids":      vmIDs,
		"vpc_id":      parseInt(vpcID),
		"customer_id": parseInt(r.customerID),
	}
	_, err := r.client.DoMethod(ctx, method, backupScheduleVMsPath(vpcID, id), body)
	return err
}

// readInto refreshes m, including the VM membership. Returns false when the
// schedule is gone so the caller can drop it from state.
func (r *BackupSchedulerResource) readInto(ctx context.Context, m *BackupSchedulerResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}
	id := m.ID.ValueString()

	raw, err := r.client.DoMethod(ctx, http.MethodGet, backupSchedulePath(vpcID, id), nil)
	if err != nil {
		if isBackupSchedulerNotFound(err) {
			return false
		}
		diags.AddError("Backup scheduler read failed", err.Error())
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
		diags.AddError("Backup scheduler decode failed", err.Error())
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

	// Membership comes from the sub-resource; without it a VM added or removed
	// outside Terraform would never show up as drift.
	vmIDs, err := r.readVMIDs(ctx, vpcID, id)
	if err != nil {
		diags.AddError("Cannot read backup scheduler members", err.Error())
		return true
	}
	set, setDiags := types.SetValueFrom(ctx, types.StringType, vmIDs)
	diags.Append(setDiags...)
	if !setDiags.HasError() {
		m.VMIDs = set
	}
	return true
}

// readVMIDs returns the VMs that are actually members of the schedule. A VM
// being removed stays in this list with status REMOVING_VM until the backend
// finishes; counting it as a member would make the apply that removed it report
// the VM as still present.
func (r *BackupSchedulerResource) readVMIDs(ctx context.Context, vpcID, id string) ([]string, error) {
	raw, err := r.client.DoMethod(ctx, http.MethodGet, backupScheduleVMsPath(vpcID, id), nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data []struct {
			VMID   string `json:"vmId"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(env.Data))
	for _, v := range env.Data {
		if strings.EqualFold(v.Status, vmStatusRemoving) {
			continue
		}
		out = append(out, v.VMID)
	}
	return out, nil
}

// waitForSchedule blocks until the schedule leaves its transient state.
// Observed states: CREATING and UPDATING while work is in flight, SUCCESS when
// done — and adding or removing a VM also puts the schedule into UPDATING, so
// this one wait covers create, update and membership changes alike.
//
// No failure state has ever been observed, so anything unrecognised is treated
// as still-working and hits the timeout rather than being guessed at.
func (r *BackupSchedulerResource) waitForSchedule(ctx context.Context, vpcID, id string) error {
	deadline := time.Now().Add(backupSchedulerTimeout)
	var last string
	for {
		raw, err := r.client.DoMethod(ctx, http.MethodGet, backupSchedulePath(vpcID, id), nil)
		if err != nil {
			return err
		}
		var env struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return err
		}
		last = env.Data.Status
		if strings.EqualFold(last, scheduleStatusSuccess) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("backup schedule %s still reports status %q after %s", id, last, backupSchedulerTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// ---------- Pure helpers ----------

func extractBackupSchedulerID(raw []byte) (string, error) {
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

// diffStrings reports which of want are missing from have, and which of have
// are no longer wanted.
func diffStrings(have, want []string) (added, removed []string) {
	inHave := make(map[string]bool, len(have))
	for _, v := range have {
		inHave[v] = true
	}
	inWant := make(map[string]bool, len(want))
	for _, v := range want {
		inWant[v] = true
		if !inHave[v] {
			added = append(added, v)
		}
	}
	for _, v := range have {
		if !inWant[v] {
			removed = append(removed, v)
		}
	}
	return added, removed
}

func isBackupSchedulerNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HTTP 404") || strings.Contains(s, "ERROR.NOT.FOUND")
}
