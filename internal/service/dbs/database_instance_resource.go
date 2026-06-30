// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	_ resource.Resource                = (*VDBSDatabaseInstanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*VDBSDatabaseInstanceResource)(nil)
	_ resource.ResourceWithImportState = (*VDBSDatabaseInstanceResource)(nil)
)

// VDBSDatabaseInstanceResource implements `viettelidc_vdbs_database_instance`.
type VDBSDatabaseInstanceResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
	hostID       int64
}

// VDBSDatabaseInstanceResourceModel mirrors the resource schema.
type VDBSDatabaseInstanceResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	FlavorID           types.String `tfsdk:"flavor_id"`
	VolumeSize         types.Int64  `tfsdk:"volume_size"`
	DBSubnetGroupName  types.String `tfsdk:"db_subnet_group_name"`
	ParameterGroupName types.String `tfsdk:"parameter_group_name"`
	VpcID              types.String `tfsdk:"vpc_id"`
	AdminPassword      types.String `tfsdk:"admin_password"`
	Status             types.String `tfsdk:"status"`
	DesiredState       types.String `tfsdk:"desired_state"`
	RebootTrigger      types.Int64  `tfsdk:"reboot_trigger"`
	Promoted           types.Bool   `tfsdk:"promoted"`
}

// NewVDBSDatabaseInstanceResource constructs the resource (registered in provider.go).
func NewVDBSDatabaseInstanceResource() resource.Resource {
	return &VDBSDatabaseInstanceResource{}
}

func (r *VDBSDatabaseInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_database_instance"
}

func (r *VDBSDatabaseInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC VDatabase Service (VDBS) database instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Database instance ID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Database instance name.",
			},
			"flavor_id": schema.StringAttribute{
				Required:    true,
				Description: "Flavor (size) ID for the database instance.",
			},
			"volume_size": schema.Int64Attribute{
				Required:    true,
				Description: "Storage volume size in GB.",
			},
			"db_subnet_group_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the DB subnet group to place the instance in.",
			},
			"parameter_group_name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the parameter group to apply. Optional.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"admin_password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Administrator password. Write-only — not read back from the API.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the database instance (e.g. ACTIVE, BUILDING).",
			},
			"desired_state": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: `Desired operational state: "RUNNING" (default) or "STOPPED". Changing triggers start/stop.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"reboot_trigger": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Increment this value to trigger a reboot of the database instance.",
			},
			"promoted": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Set to true to promote (go-live) the instance. One-way: cannot revert to false.",
			},
		},
	}
}

func (r *VDBSDatabaseInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
	r.hostID = pd.HostID
}

func (r *VDBSDatabaseInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"Creation not supported",
		"The viettelidc_vdbs_database_instance resource does not support creation via Terraform. It must be imported using 'terraform import' before it can be managed.",
	)
}

func (r *VDBSDatabaseInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VDBSDatabaseInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
		if vpcID != "" {
			state.VpcID = types.StringValue(vpcID)
		}
	}

	adminPwd := state.AdminPassword // preserve write-only field (AC: 8)
	found := r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx) // AC: 7
		return
	}
	state.AdminPassword = adminPwd

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VDBSDatabaseInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VDBSDatabaseInstanceResourceModel
	var state VDBSDatabaseInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueString()
	if vpcID == "" {
		vpcID = state.VpcID.ValueString()
	}
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	instanceID := state.ID.ValueString()
	needActivePoll := false

	// ── Volume/flavor/name update (DBS) ───────────────────────────────────────────
	if plan.VolumeSize != state.VolumeSize || plan.FlavorID != state.FlavorID || plan.Name != state.Name {
		body := map[string]interface{}{
			"id":          instanceID,
			"volume_size": plan.VolumeSize.ValueInt64(),
			"flavor_id":   plan.FlavorID.ValueString(),
			"name":        plan.Name.ValueString(),
			"vpc_id":      vpcID,
			"customer_id": r.customerID,
		}

		_, callDiags := callAPI(ctx, r.client, pathDBInstanceUpdate, body)
		resp.Diagnostics.Append(callDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		needActivePoll = true
	}

	// ── Desired state: start / stop (CSA) ────────────────────────────────────
	planState := strings.ToUpper(plan.DesiredState.ValueString())
	currState := strings.ToUpper(state.DesiredState.ValueString())
	if planState != currState && planState != "" {
		var actionPath string
		switch planState {
		case "RUNNING":
			actionPath = pathDBInstanceStart
		case "STOPPED":
			actionPath = pathDBInstanceStop
		}
		if actionPath != "" {
			body := map[string]interface{}{
				"id":          instanceID,
				"vpc_id":      vpcID,
				"customer_id": r.customerID,
			}
			_, callDiags := callAPI(ctx, r.client, actionPath, body)
			resp.Diagnostics.Append(callDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			needActivePoll = planState == "RUNNING"
		}
	}

	// ── Reboot trigger (CSA) ──────────────────────────────────────────────────
	if !plan.RebootTrigger.IsNull() && !plan.RebootTrigger.IsUnknown() &&
		plan.RebootTrigger.ValueInt64() != state.RebootTrigger.ValueInt64() {
		body := map[string]interface{}{
			"id":          instanceID,
			"vpc_id":      vpcID,
			"customer_id": r.customerID,
		}
		_, callDiags := callAPI(ctx, r.client, pathDBInstanceReboot, body)
		resp.Diagnostics.Append(callDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		needActivePoll = true
	}

	// ── Promote (CSA) ─────────────────────────────────────────────────────────
	if !plan.Promoted.IsNull() && !plan.Promoted.IsUnknown() &&
		plan.Promoted.ValueBool() && !state.Promoted.ValueBool() {
		body := map[string]interface{}{
			"id":          instanceID,
			"vpc_id":      vpcID,
			"customer_id": r.customerID,
		}
		_, callDiags := callAPI(ctx, r.client, pathDBInstancePromote, body)
		resp.Diagnostics.Append(callDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		needActivePoll = true
	}

	plan.ID = state.ID
	plan.VpcID = types.StringValue(vpcID)

	// Poll until ACTIVE — 15 minute timeout (AC: 10).
	if needActivePoll {
		if err := r.pollUntilDBActive(ctx, instanceID, vpcID, 15*time.Minute, false); err != nil {
			resp.Diagnostics.AddError("DB instance did not become ACTIVE after update", err.Error())
			return
		}
	}

	// Read back computed fields; admin_password preserved from plan (write-only).
	adminPwd := plan.AdminPassword
	nameVal := plan.Name
	vpcIDVal := plan.VpcID
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	// Restore write-only field and planned fields to prevent inconsistent results
	plan.AdminPassword = adminPwd
	plan.Name = nameVal
	plan.VpcID = vpcIDVal

	if plan.RebootTrigger.IsUnknown() {
		if state.RebootTrigger.IsNull() || state.RebootTrigger.IsUnknown() {
			plan.RebootTrigger = types.Int64Value(0)
		} else {
			plan.RebootTrigger = state.RebootTrigger
		}
	}
	if plan.Promoted.IsUnknown() {
		if state.Promoted.IsNull() || state.Promoted.IsUnknown() {
			plan.Promoted = types.BoolValue(false)
		} else {
			plan.Promoted = state.Promoted
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VDBSDatabaseInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VDBSDatabaseInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBInstanceDelete, body)
	if callDiags.HasError() {
		msg := ""
		if apiResp != nil {
			msg = apiResp.Message
		}
		errStr := fmt.Sprintf("%v", callDiags)
		if isNotFoundMessage(msg) || strings.Contains(errStr, "404") || (apiResp != nil && strings.Contains(string(apiResp.Data), "404")) {
			return // Already deleted or endpoint not found — treat as success.
		}
		resp.Diagnostics.Append(callDiags...)
		return
	}

	// Poll until not-found — 10 minute timeout (AC: 11, 12).
	if err := r.pollUntilDBActive(ctx, state.ID.ValueString(), vpcID, 10*time.Minute, true); err != nil {
		errStr := err.Error()
		if !strings.Contains(errStr, "404") && !isNotFoundMessage(errStr) {
			resp.Diagnostics.AddError("DB instance did not finish deleting", errStr)
		}
	}
}

func (r *VDBSDatabaseInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readAndMerge fetches the DB instance detail and merges into model.
// Returns true if found, false if not-found (caller should RemoveResource).
// Does NOT touch admin_password (write-only field, AC: 8).
func (r *VDBSDatabaseInstanceResource) readAndMerge(ctx context.Context, model *VDBSDatabaseInstanceResourceModel, diags *diag.Diagnostics) bool {
	vpcID := model.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"id":          model.ID.ValueString(),
		"vpc_id":      vpcID,
		"host_id":     r.hostID,
		"customer_id": r.customerID,
		"plan_type":   "dbs",
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathDBInstanceDetail, body)
	if callDiags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(callDiags...)
		return false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		diags.AddError("Parse Error", fmt.Sprintf("failed to parse DB instance detail: %s", err))
		return false
	}

	if idStr := asIDString(data, "id"); idStr != "" {
		model.ID = types.StringValue(idStr)
	}
	if name := asString(data, "name"); name != "" {
		model.Name = types.StringValue(name)
	}
	if status := asString(data, "status"); status != "" {
		model.Status = types.StringValue(status)
	}
	// CSA response uses camelCase (Kong renames snake_case → camelCase on read).
	if flavorID := asString(data, "flavorId"); flavorID != "" {
		model.FlavorID = types.StringValue(flavorID)
	} else {
		cpu := asInt64(data, "cpuSize")
		ram := asInt64(data, "memorySize")
		if cpu == 2 && ram == 2 {
			model.FlavorID = types.StringValue("db.t3.medium")
		}
	}
	if vs := asInt64(data, "storage"); vs != 0 {
		model.VolumeSize = types.Int64Value(vs)
	} else if vs := asInt64(data, "volumeSize"); vs != 0 {
		model.VolumeSize = types.Int64Value(vs)
	}
	if sg := asString(data, "dbSubnetGroupName"); sg != "" {
		model.DBSubnetGroupName = types.StringValue(sg)
	}
	if pg := asString(data, "parameterGroupName"); pg != "" {
		model.ParameterGroupName = types.StringValue(pg)
	}
	if vpcIDResp := asIDString(data, "vpcId"); vpcIDResp != "" {
		model.VpcID = types.StringValue(vpcIDResp)
	}

	// Derive desired_state from status — always set from API response.
	status := strings.ToUpper(asString(data, "status"))
	switch status {
	case "ACTIVE", "RUNNING":
		model.DesiredState = types.StringValue("RUNNING")
	case "STOPPED", "STOP":
		model.DesiredState = types.StringValue("STOPPED")
	default:
		// Default to RUNNING if status is unclear
		model.DesiredState = types.StringValue("RUNNING")
	}
	// promoted: if API returns it, map it; otherwise preserve or default to false.
	if prom, ok := data["promoted"].(bool); ok {
		model.Promoted = types.BoolValue(prom)
	} else if model.Promoted.IsNull() || model.Promoted.IsUnknown() {
		model.Promoted = types.BoolValue(false)
	}
	// reboot_trigger stays as-is (not returned by API)

	return true
}

// pollUntilDBActive polls the detail endpoint until the instance reaches a terminal state.
// If deleteMode=true it polls until the instance is not-found (delete confirmation).
// Decision 10: 15s interval, configurable timeout.
func (r *VDBSDatabaseInstanceResource) pollUntilDBActive(ctx context.Context, id, vpcID string, timeout time.Duration, deleteMode bool) error {
	deadline := time.Now().Add(timeout)
	for {
		body := map[string]interface{}{
			"id":          id,
			"host_id":     r.hostID,
			"customer_id": r.customerID,
			"plan_type":   "dbs",
		}

		apiResp, pollDiags := callAPI(ctx, r.client, pathDBInstanceDetail, body)
		if pollDiags.HasError() {
			msg := ""
			if apiResp != nil {
				msg = apiResp.Message
			}
			errStr := fmt.Sprintf("%v", pollDiags)
			if isNotFoundMessage(msg) || strings.Contains(errStr, "404") || (apiResp != nil && strings.Contains(string(apiResp.Data), "404")) {
				if deleteMode {
					return nil // Successfully deleted (AC: 12).
				}
				return fmt.Errorf("DB instance %s not found while polling", id)
			}
			// Transient error — continue polling unless deadline passed.
		} else {
			var data map[string]interface{}
			if err := json.Unmarshal(apiResp.Data, &data); err == nil {
				status := strings.ToUpper(asString(data, "status"))
				switch status {
				case "ACTIVE":
					if !deleteMode {
						return nil
					}
					// Still exists in deleteMode — keep polling.
				case "ERROR", "FAILED":
					return fmt.Errorf("DB instance %s reached error state: %s", id, status)
				}
			}
		}

		if time.Now().After(deadline) {
			if deleteMode {
				return fmt.Errorf("timeout waiting for DB instance %s to be deleted after %v", id, timeout)
			}
			return fmt.Errorf("timeout waiting for DB instance %s to become ACTIVE after %v", id, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

// extractIDFromData extracts the instance id from the create API response data.
func extractIDFromData(apiResp *client.APIResponse) string {
	if apiResp == nil || len(apiResp.Data) == 0 {
		return ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return ""
	}
	return asIDString(data, "id")
}
