// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package dbs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/client"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

var (
	_ resource.Resource                = &securityGroupRuleResource{}
	_ resource.ResourceWithConfigure   = &securityGroupRuleResource{}
	_ resource.ResourceWithImportState = &securityGroupRuleResource{}
)

// sgRuleLocks serialises concurrent rule operations on the same security group.
var sgRuleLocks sync.Map // map[sgID string]*sync.Mutex

func NewVDBSSecurityGroupRuleResource() resource.Resource {
	return &securityGroupRuleResource{}
}

type securityGroupRuleResource struct {
	clientData *providerdata.ProviderData
}

type SecurityGroupRuleResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	Type            types.String `tfsdk:"type"`
	ProtocolName    types.String `tfsdk:"protocol_name"`
	Port            types.String `tfsdk:"port"`
	Name            types.String `tfsdk:"name"`
	Source          types.String `tfsdk:"source"`
	SourceIP        types.String `tfsdk:"source_ip"`
	Destination     types.String `tfsdk:"destination"`
	Status          types.String `tfsdk:"status"`
	HostID          types.Int64  `tfsdk:"host_id"`
}

func (r *securityGroupRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vdbs_security_group_rule"
}

func (r *securityGroupRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	r.clientData = clientData
}

func (r *securityGroupRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provides a VDBS Security Group Rule resource to manage inbound rules for Database Service.\n\n" +
			"> **Note:** VDBS currently only supports managing inbound rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the security group rule.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"security_group_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the VDBS security group to attach the rule to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Rule type (e.g., `All TCP`, `Other Protocol - IPv4`).",
				Required:            true,
			},
			"protocol_name": schema.StringAttribute{
				MarkdownDescription: "Protocol name (e.g., `TCP`, `UDP`, `Any`).",
				Required:            true,
			},
			"port": schema.StringAttribute{
				MarkdownDescription: "Port or port range (e.g., `3306`, `Any`).",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the rule.",
				Optional:            true,
				Computed:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Source type (e.g., `anywhere`, `custom`). Default is `anywhere`.",
				Optional:            true,
				Computed:            true,
			},
			"source_ip": schema.StringAttribute{
				MarkdownDescription: "Source IP CIDR (e.g., `0.0.0.0/0`).",
				Required:            true,
			},
			"destination": schema.StringAttribute{
				MarkdownDescription: "Destination type. Default is `custom`.",
				Optional:            true,
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current status of the rule.",
				Computed:            true,
			},
			"host_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the host. Inherited from provider if not set.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *securityGroupRuleResource) buildPayload(plan *SecurityGroupRuleResourceModel, action string, sourceRuleID string) map[string]interface{} {
	dir := "in"

	payload := map[string]interface{}{
		"rule_type":          plan.Type.ValueString(),
		"type":               plan.Type.ValueString(),
		"protocol_name":      plan.ProtocolName.ValueString(),
		"protocolName":       plan.ProtocolName.ValueString(),
		"port":               plan.Port.ValueString(),
		"source":             plan.Source.ValueString(),
		"destination":        "custom",
		"source_ip":          plan.SourceIP.ValueString(),
		"sourceIP":           plan.SourceIP.ValueString(),
		"action":             action,
		"total_rule_add":     1,
		"totalRuleAdd":       1,
		"security_group_id":  plan.SecurityGroupID.ValueString(),
		"vttSecurityGroupId": plan.SecurityGroupID.ValueString(),
		"name":               plan.Name.ValueString(),
		"is_valid":           true,
		"isValid":            true,
		"direction":          dir,
		"plan_type":          "dbs",
		"planType":           "dbs",
	}

	if !plan.Destination.IsNull() && !plan.Destination.IsUnknown() {
		payload["destination"] = plan.Destination.ValueString()
	}

	if sourceRuleID != "" {
		payload["id"] = sourceRuleID
	}

	return payload
}

func (r *securityGroupRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostID := r.clientData.HostID
	if !plan.HostID.IsNull() && !plan.HostID.IsUnknown() {
		hostID = plan.HostID.ValueInt64()
	}
	if hostID == 0 {
		resp.Diagnostics.AddError("Missing Host ID", "host_id must be configured.")
		return
	}

	// Serialize per-SG
	sgMuRaw, _ := sgRuleLocks.LoadOrStore(plan.SecurityGroupID.ValueString(), &sync.Mutex{})
	sgMu := sgMuRaw.(*sync.Mutex)
	sgMu.Lock()
	defer sgMu.Unlock()

	payload := r.buildPayload(&plan, "New", "")
	payload["host_id"] = hostID
	payload["hostId"] = hostID

	if custIDInt, err := strconv.Atoi(r.clientData.CustomerID); err == nil {
		payload["customer_id"] = custIDInt
	}
	payload["customerId"] = r.clientData.CustomerID

	// Convert security_group_id to int for Kong
	if sgIDInt, err := strconv.Atoi(plan.SecurityGroupID.ValueString()); err == nil {
		payload["security_group_id"] = sgIDInt
	}

	raw, err := r.clientData.Client.DoMethod(ctx, "POST", pathDBSGRuleCreate, payload)

	var parsedResp *client.APIResponse
	parsedResp, err = client.ParseAPIResponse(raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid API Response", "Could not parse response JSON")
		return
	}
	if !parsedResp.IsSuccess() {
		resp.Diagnostics.AddError("Loi API", fmt.Sprintf("%s\n(path=%s code=%v)", parsedResp.Message, pathDBSGRuleCreate, parsedResp.Code))
		return
	}

	var rule map[string]interface{}
	for i := 0; i < 30; i++ { // 2.5 minutes timeout
		time.Sleep(5 * time.Second)
		rFound, lookupDiags := r.findRuleInList(ctx, plan.SecurityGroupID.ValueString())
		if !lookupDiags.HasError() && rFound != nil {
			rule = rFound
			break
		}
	}

	if rule == nil {
		resp.Diagnostics.AddError("Creation Error", "Could not locate created rule after API call (timeout).")
		return
	}

	r.updateStateFromMap(&plan, rule, hostID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *securityGroupRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostID := r.clientData.HostID
	if !state.HostID.IsNull() && !state.HostID.IsUnknown() {
		hostID = state.HostID.ValueInt64()
	}

	rule, diags := r.findRuleByID(ctx, state.SecurityGroupID.ValueString(), state.ID.ValueString())
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
	if rule == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.updateStateFromMap(&state, rule, hostID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *securityGroupRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostID := r.clientData.HostID
	if !plan.HostID.IsNull() && !plan.HostID.IsUnknown() {
		hostID = plan.HostID.ValueInt64()
	}

	// Serialize per-SG
	sgMuRaw, _ := sgRuleLocks.LoadOrStore(plan.SecurityGroupID.ValueString(), &sync.Mutex{})
	sgMu := sgMuRaw.(*sync.Mutex)
	sgMu.Lock()
	defer sgMu.Unlock()

	payload := r.buildPayload(&plan, "Edit", state.ID.ValueString())
	payload["host_id"] = hostID
	payload["hostId"] = hostID

	if custIDInt, err := strconv.Atoi(r.clientData.CustomerID); err == nil {
		payload["customer_id"] = custIDInt
	}
	payload["customerId"] = r.clientData.CustomerID

	// Convert security_group_id to int for Kong
	if sgIDInt, err := strconv.Atoi(plan.SecurityGroupID.ValueString()); err == nil {
		payload["security_group_id"] = sgIDInt
	}

	_, diags := callAPI(ctx, r.clientData.Client, pathDBSGRuleUpdate, payload)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, lookupDiags := r.findRuleByID(ctx, plan.SecurityGroupID.ValueString(), state.ID.ValueString())
	if lookupDiags.HasError() {
		resp.Diagnostics.Append(lookupDiags...)
		return
	}
	if rule != nil {
		r.updateStateFromMap(&plan, rule, hostID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *securityGroupRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hostID := r.clientData.HostID
	if !state.HostID.IsNull() && !state.HostID.IsUnknown() {
		hostID = state.HostID.ValueInt64()
	}

	// Serialize per-SG
	sgMuRaw, _ := sgRuleLocks.LoadOrStore(state.SecurityGroupID.ValueString(), &sync.Mutex{})
	sgMu := sgMuRaw.(*sync.Mutex)
	sgMu.Lock()
	defer sgMu.Unlock()

	payload := map[string]interface{}{
		"security_group_id":  state.SecurityGroupID.ValueString(),
		"vttSecurityGroupId": state.SecurityGroupID.ValueString(),
		"security_rule_id":   state.ID.ValueString(),
		"vttSecurityRuleId":  state.ID.ValueString(),
		"id":                 state.ID.ValueString(),
		"host_id":            hostID,
		"hostId":             hostID,
		"plan_type":          "dbs",
		"planType":           "dbs",
	}

	if custIDInt, err := strconv.Atoi(r.clientData.CustomerID); err == nil {
		payload["customer_id"] = custIDInt
	}
	payload["customerId"] = r.clientData.CustomerID

	_, diags := callAPI(ctx, r.clientData.Client, pathDBSGRuleDelete, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
	}

	// Poll until deleted
	for i := 0; i < 30; i++ { // 2.5 mins timeout
		time.Sleep(5 * time.Second)
		rFound, lookupDiags := r.findRuleByID(ctx, state.SecurityGroupID.ValueString(), state.ID.ValueString())
		if !lookupDiags.HasError() && rFound == nil {
			return // successfully deleted
		}
	}
	resp.Diagnostics.AddError("Delete Timeout", "Security group rule was not deleted within the expected time.")
}

func (r *securityGroupRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import Format",
			"Import ID must be in the format `security_group_id:rule_id`",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("security_group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func (r *securityGroupRuleResource) findRuleByID(ctx context.Context, sgID, ruleID string) (map[string]interface{}, diag.Diagnostics) {
	payload := map[string]interface{}{
		"pageIndex":          0,
		"pageSize":           9999,
		"vttSecurityGroupId": sgID,
		"security_group_id":  sgID,
		"plan_type":          "dbs",
		"planType":           "dbs",
		"host_id":            r.clientData.HostID,
		"hostId":             r.clientData.HostID,
	}

	if custIDInt, err := strconv.Atoi(r.clientData.CustomerID); err == nil {
		payload["customer_id"] = custIDInt
	}
	payload["customerId"] = r.clientData.CustomerID

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathDBSGRuleInboundList, payload)
	if diags.HasError() {
		return nil, diags
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		var parseDiags diag.Diagnostics
		parseDiags.AddError("Parse Error", "Failed to parse inbound rules list")
		return nil, parseDiags
	}

	itemsVal, ok := responseMap["items"]
	if !ok || itemsVal == nil {
		return nil, nil
	}
	itemsList, ok := itemsVal.([]interface{})
	if !ok {
		return nil, nil
	}

	for _, itemRaw := range itemsList {
		if itemMap, ok := itemRaw.(map[string]interface{}); ok {
			idVal := asString(itemMap, "vttSecurityRuleId")
			if idVal == "" {
				idVal = asString(itemMap, "id")
			}
			if idVal == ruleID {
				return itemMap, nil
			}
		}
	}
	return nil, nil
}

func (r *securityGroupRuleResource) findRuleInList(ctx context.Context, sgID string) (map[string]interface{}, diag.Diagnostics) {
	payload := map[string]interface{}{
		"pageIndex":          0,
		"pageSize":           9999,
		"vttSecurityGroupId": sgID,
		"security_group_id":  sgID,
		"plan_type":          "dbs",
		"planType":           "dbs",
		"host_id":            r.clientData.HostID,
		"hostId":             r.clientData.HostID,
	}

	if custIDInt, err := strconv.Atoi(r.clientData.CustomerID); err == nil {
		payload["customer_id"] = custIDInt
	}
	payload["customerId"] = r.clientData.CustomerID

	apiResp, diags := callAPI(ctx, r.clientData.Client, pathDBSGRuleInboundList, payload)
	if diags.HasError() {
		return nil, diags
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &responseMap); err != nil {
		var parseDiags diag.Diagnostics
		parseDiags.AddError("Parse Error", "Failed to parse inbound rules list")
		return nil, parseDiags
	}

	itemsVal, ok := responseMap["items"]
	if !ok || itemsVal == nil {
		return nil, nil
	}
	itemsList, ok := itemsVal.([]interface{})
	if !ok || len(itemsList) == 0 {
		return nil, nil
	}

	var maxID int64 = -1
	var latestRule map[string]interface{}
	for _, itemRaw := range itemsList {
		if itemMap, ok := itemRaw.(map[string]interface{}); ok {
			idStr := asString(itemMap, "vttSecurityRuleId")
			if idStr == "" {
				idStr = asString(itemMap, "id")
			}
			idNum, _ := strconv.ParseInt(idStr, 10, 64)
			if idNum > maxID {
				maxID = idNum
				latestRule = itemMap
			}
		}
	}
	return latestRule, nil
}

func (r *securityGroupRuleResource) updateStateFromMap(state *SecurityGroupRuleResourceModel, rule map[string]interface{}, hostID int64) {
	idVal := asString(rule, "vttSecurityRuleId")
	if idVal == "" {
		idVal = asString(rule, "id")
	}
	state.ID = types.StringValue(idVal)
	state.Type = types.StringValue(asString(rule, "type"))
	state.ProtocolName = types.StringValue(asString(rule, "protocolName"))
	state.Port = types.StringValue(asString(rule, "port"))

	if name := asString(rule, "name"); name != "" {
		state.Name = types.StringValue(name)
	}

	if source := asString(rule, "source"); source != "" {
		state.Source = types.StringValue(source)
	}
	if sourceIP := asString(rule, "sourceIP"); sourceIP != "" {
		state.SourceIP = types.StringValue(sourceIP)
	}
	if destination := asString(rule, "destination"); destination != "" {
		state.Destination = types.StringValue(destination)
	}

	state.Status = types.StringValue(asString(rule, "status"))
	state.HostID = types.Int64Value(hostID)
}
