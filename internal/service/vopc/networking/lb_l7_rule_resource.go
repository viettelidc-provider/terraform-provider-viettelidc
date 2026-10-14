// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                   = (*L7RuleResource)(nil)
	_ resource.ResourceWithConfigure      = (*L7RuleResource)(nil)
	_ resource.ResourceWithImportState    = (*L7RuleResource)(nil)
	_ resource.ResourceWithValidateConfig = (*L7RuleResource)(nil)
)

var digitsOnlyRegex = regexp.MustCompile(`^\d+$`)

type L7RuleResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7RuleResource() resource.Resource {
	return &L7RuleResource{}
}

type L7RuleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	VpcID       types.Int64  `tfsdk:"vpc_id"`
	PolicyID    types.Int64  `tfsdk:"policy_id"`
	ListenerID  types.Int64  `tfsdk:"listener_id"`
	Type        types.String `tfsdk:"type"`
	CompareType types.String `tfsdk:"compare_type"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Status      types.String `tfsdk:"status"`
}

type l7RuleDetailResponse struct {
	ID                        interface{} `json:"id"`
	VttLoadBalancerL7RuleId   int64       `json:"vttLoadBalancerL7RuleId"`
	VttLoadBalancerL7PolicyId int64       `json:"vttLoadBalancerL7PolicyId"`
	VttLoadBalancerListenerId int64       `json:"vttLoadBalancerListenerId"`
	Type                      string      `json:"type"`
	CompareType               string      `json:"compareType"`
	RuleKey                   string      `json:"ruleKey"`
	Value                     string      `json:"value"`
	Priority                  *int64      `json:"priority"`
	Status                    string      `json:"status"`
}

func (r *L7RuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_rule"
}

func (r *L7RuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ViettelIDC vLB Layer 7 Rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Internal CMP ID of the L7 rule.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.Int64Attribute{
				Required:    true,
				Description: "VPC ID owning the Load Balancer.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"policy_id": schema.Int64Attribute{
				Required:    true,
				Description: "ID of the parent L7 policy.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"listener_id": schema.Int64Attribute{
				Computed:    true,
				Description: "ID of the Listener resolved from parent policy.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Rule type: HOST_NAME, PATH, FILE_TYPE, HEADER, or COOKIE.",
				Validators: []validator.String{
					stringvalidator.OneOf("HOST_NAME", "PATH", "FILE_TYPE", "HEADER", "COOKIE"),
				},
			},
			"compare_type": schema.StringAttribute{
				Required:    true,
				Description: "Comparison type: EQUAL_TO, STARTS_WITH, ENDS_WITH, CONTAINS, or REGEX.",
				Validators: []validator.String{
					stringvalidator.OneOf("EQUAL_TO", "STARTS_WITH", "ENDS_WITH", "CONTAINS", "REGEX"),
				},
			},
			"key": schema.StringAttribute{
				Optional:    true,
				Description: "Header or cookie name. Required for HEADER and COOKIE types; forbidden for others.",
			},
			"value": schema.StringAttribute{
				Required:    true,
				Description: "Value to compare against (up to 255 characters).",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Lifecycle status of the rule (pending, success, active, error).",
			},
		},
	}
}

func (r *L7RuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

// ValidateConfig enforces Class A & B client-side validations (Sections 5.2 and 9).
func (r *L7RuleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg L7RuleResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Type.IsUnknown() || cfg.CompareType.IsUnknown() {
		return
	}

	ruleType := strings.ToUpper(cfg.Type.ValueString())
	compareType := strings.ToUpper(cfg.CompareType.ValueString())

	// FILE_TYPE only allows EQUAL_TO or REGEX
	if ruleType == "FILE_TYPE" && compareType != "EQUAL_TO" && compareType != "REGEX" {
		resp.Diagnostics.AddAttributeError(
			path.Root("compare_type"),
			"Invalid Compare Type for FILE_TYPE",
			"FILE_TYPE rule only supports EQUAL_TO or REGEX compare types",
		)
	}

	// Key validation
	if ruleType == "HEADER" || ruleType == "COOKIE" {
		if cfg.Key.IsNull() || (!cfg.Key.IsUnknown() && strings.TrimSpace(cfg.Key.ValueString()) == "") {
			resp.Diagnostics.AddAttributeError(
				path.Root("key"),
				"Missing Required Key",
				fmt.Sprintf("key is required for rule type %s", ruleType),
			)
		} else if !cfg.Key.IsUnknown() {
			k := cfg.Key.ValueString()
			if len(k) > 255 {
				resp.Diagnostics.AddAttributeError(path.Root("key"), "Key Too Long", "key must not exceed 255 characters")
			}
			if strings.ContainsAny(k, " \t\r\n") {
				resp.Diagnostics.AddAttributeError(path.Root("key"), "Invalid Key", "key must not contain whitespace characters")
			}
			for i := range k {
				if k[i] < 0x20 || k[i] == 0x7F {
					resp.Diagnostics.AddAttributeError(path.Root("key"), "Invalid Key", "key must not contain control characters")
					break
				}
			}
			if strings.ContainsAny(k, ";,=\"") {
				resp.Diagnostics.AddAttributeError(path.Root("key"), "Invalid Key", "key must not contain semicolons, commas, equals, or quotes")
			}
		}
	} else {
		if !cfg.Key.IsNull() && !cfg.Key.IsUnknown() && cfg.Key.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("key"),
				"Invalid Attribute Combination",
				fmt.Sprintf("key must not be set for rule type %s", ruleType),
			)
		}
	}

	// Value validation
	if !cfg.Value.IsUnknown() {
		val := cfg.Value.ValueString()
		if len(val) == 0 {
			resp.Diagnostics.AddAttributeError(path.Root("value"), "Missing Value", "value must not be empty")
		}
		if len(val) > 255 {
			resp.Diagnostics.AddAttributeError(path.Root("value"), "Value Too Long", "value must not exceed 255 characters")
		}
		if strings.ContainsAny(val, " \t\r\n") {
			resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid Value", "value must not contain whitespace characters")
		}
		for i := range val {
			if val[i] < 0x20 || val[i] == 0x7F {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid Value", "value must not contain control characters")
				break
			}
		}

		switch ruleType {
		case "PATH":
			if !strings.HasPrefix(val, "/") {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid PATH Value", "PATH value must start with '/'")
			}
		case "FILE_TYPE":
			if strings.HasPrefix(val, ".") {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid FILE_TYPE Value", "FILE_TYPE value must not start with '.' (use 'jpg', not '.jpg')")
			}
		case "HOST_NAME":
			if strings.Contains(strings.ToLower(val), "http://") || strings.Contains(strings.ToLower(val), "https://") {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid HOST_NAME Value", "HOST_NAME value must not contain 'http://' or 'https://'")
			}
			if digitsOnlyRegex.MatchString(val) {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "Invalid HOST_NAME Value", "HOST_NAME value must not consist solely of digits")
			}
		}

		if compareType == "REGEX" {
			if _, err := regexp.Compile(val); err != nil {
				resp.Diagnostics.AddAttributeWarning(path.Root("value"), "Regex Syntax Warning", fmt.Sprintf("value is not a valid Go RE2 regex (%v). If this is a valid PCRE regex, Octavia may accept it.", err))
			}
		}
	}
}

func (r *L7RuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan L7RuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	policyID := plan.PolicyID.ValueInt64()

	// 1. Fetch parent policy detail to ensure it exists and is success/active
	policy, gone, err := r.getParentPolicyDetail(ctx, vpcID, policyID)
	if gone || policy == nil {
		resp.Diagnostics.AddError("Parent Policy Not Found", fmt.Sprintf("Parent L7 Policy %d not found or has been deleted", policyID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Parent Policy Detail Error", err.Error())
		return
	}

	policyStatus := strings.ToLower(policy.Status)
	if policyStatus == "error" || policyStatus == "deleting" {
		resp.Diagnostics.AddError(
			"Parent Policy Invalid Status",
			fmt.Sprintf("Parent L7 Policy %d is in '%s' state; cannot create rule under it", policyID, policy.Status),
		)
		return
	}
	if policyStatus == "pending" {
		if err := r.waitPolicySuccess(ctx, vpcID, policyID, 5*time.Minute); err != nil {
			resp.Diagnostics.AddError("Parent Policy Pending Timeout", err.Error())
			return
		}
	}

	listenerID := policy.VttLoadBalancerListenerId
	plan.ListenerID = types.Int64Value(listenerID)

	// Fetch listener to obtain Load Balancer ID for mutex locking
	listener, err := r.getListenerDetail(ctx, vpcID, listenerID)
	if err != nil || listener == nil {
		resp.Diagnostics.AddError("Listener Validation Error", fmt.Sprintf("Listener %d for policy %d could not be found", listenerID, policyID))
		return
	}
	lbID := listener.VttLoadBalancerId

	// 2. Lock Load Balancer
	unlock := LockLB(lbID)
	defer unlock()

	ruleType := normalizeRuleType(plan.Type.ValueString())
	compareType := normalizeCompareType(plan.CompareType.ValueString())

	// 3. Attempt creation up to 3 times (Section 8.4)
	var createdID int64
	for attempt := 1; attempt <= 3; attempt++ {
		// NEVER send vttLoadBalancerL7RuleId in create request (avoids alias ID trap)
		body := map[string]interface{}{
			"vpcId":                     vpcID,
			"vpc_id":                    vpcID,
			"customerId":                r.customerID,
			"customer_id":               r.customerID,
			"vttLoadBalancerListenerId": listenerID,
			"listener_id":               listenerID,
			"vttLoadBalancerL7PolicyId": policyID,
			"policy_id":                 policyID,
			"type":                      ruleType,
			"compareType":               compareType,
			"compare_type":              compareType,
			"value":                     plan.Value.ValueString(),
		}
		if ruleType == "HEADER" || ruleType == "COOKIE" {
			k := plan.Key.ValueString()
			body["ruleKey"] = k
			body["key"] = k
		}

		apiResp, diags := callAPI(ctx, r.client, pathL7RuleCreate, body)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		var createResp struct {
			ID     interface{} `json:"id"`
			Status bool        `json:"status"`
			TaskID interface{} `json:"taskId"`
		}
		if err := json.Unmarshal(apiResp.Data, &createResp); err != nil {
			resp.Diagnostics.AddError("Parse Create Response Failed", err.Error())
			return
		}

		createdID = parseID(createResp.ID)
		if createdID <= 0 {
			resp.Diagnostics.AddError("Invalid Create Response", "API returned non-positive rule ID")
			return
		}

		// Save state immediately
		plan.ID = types.StringValue(strconv.FormatInt(createdID, 10))
		resp.State.Set(ctx, &plan)

		// Poll status until success, active, or error
		st, pollErr := r.pollRuleStatus(ctx, vpcID, createdID, []string{"success", "active", "error"}, 15*time.Minute)
		if pollErr == nil && (st == "success" || st == "active") {
			break
		}

		tflog.Warn(ctx, fmt.Sprintf("L7 Rule %d attempt %d failed with status %s, cleaning up before retry", createdID, attempt, st))

		// Clean up failed rule
		delBody := map[string]interface{}{
			"vpcId":                     vpcID,
			"vpc_id":                    vpcID,
			"customerId":                r.customerID,
			"customer_id":               r.customerID,
			"vttLoadBalancerL7RuleId":   createdID,
			"id":                        createdID,
			"vttLoadBalancerL7PolicyId": policyID,
			"vttLoadBalancerListenerId": listenerID,
		}
		_, _ = callAPI(ctx, r.client, pathL7RuleDelete, delBody)
		_ = r.pollRuleGone(ctx, vpcID, createdID, 5*time.Minute)
		resp.State.RemoveResource(ctx)

		if attempt == 3 {
			resp.Diagnostics.AddError(
				"L7 Rule Creation Failed",
				fmt.Sprintf("Rule %d entered error state after 3 attempts (likely Octavia 409 conflict, rule parameter rejection, or parent policy not ready)", createdID),
			)
			return
		}

		time.Sleep(time.Duration(attempt*5) * time.Second)
	}

	// 4. Read back final state
	r.readRule(ctx, vpcID, createdID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *L7RuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state L7RuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueInt64()
	ruleID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || ruleID <= 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	gone := r.readRule(ctx, vpcID, ruleID, &state, &resp.Diagnostics)
	if gone {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *L7RuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan L7RuleResourceModel
	var state L7RuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	ruleID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || ruleID <= 0 {
		resp.Diagnostics.AddError("Invalid Rule ID", "Current state has invalid rule ID")
		return
	}

	detail, gone, err := r.getRuleDetail(ctx, vpcID, ruleID)
	if gone || detail == nil {
		resp.Diagnostics.AddError("Rule Not Found", fmt.Sprintf("L7 Rule %d not found on backend", ruleID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Rule Detail Read Error", err.Error())
		return
	}

	currentStatus := strings.ToLower(detail.Status)
	if currentStatus == "error" {
		resp.Diagnostics.AddError(
			"Cannot Update Rule in Error State",
			fmt.Sprintf("L7 Rule %d is in error state. Terraform cannot update it; use 'terraform apply -replace=...' to recreate it.", ruleID),
		)
		return
	}
	if currentStatus == "pending" {
		st, pollErr := r.pollRuleStatus(ctx, vpcID, ruleID, []string{"success", "active", "error"}, 10*time.Minute)
		if pollErr != nil || st == "error" {
			resp.Diagnostics.AddError("Rule Update Precondition Failed", fmt.Sprintf("Rule %d did not settle to success before update (status=%s)", ruleID, st))
			return
		}
	}

	policyID := plan.PolicyID.ValueInt64()
	listenerID := detail.VttLoadBalancerListenerId

	listener, err := r.getListenerDetail(ctx, vpcID, listenerID)
	if err != nil || listener == nil {
		resp.Diagnostics.AddError("Listener Validation Error", fmt.Sprintf("Listener %d could not be fetched", listenerID))
		return
	}
	lbID := listener.VttLoadBalancerId

	unlock := LockLB(lbID)
	defer unlock()

	ruleType := normalizeRuleType(plan.Type.ValueString())
	compareType := normalizeCompareType(plan.CompareType.ValueString())

	// MUST SEND ALL THREE IDs in update (Section 5 & 10 #1)
	updateBody := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerL7RuleId":   ruleID,
		"id":                        ruleID,
		"vttLoadBalancerL7PolicyId": policyID,
		"policy_id":                 policyID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
		"type":                      ruleType,
		"compareType":               compareType,
		"compare_type":              compareType,
		"value":                     plan.Value.ValueString(),
	}
	if ruleType == "HEADER" || ruleType == "COOKIE" {
		k := plan.Key.ValueString()
		updateBody["ruleKey"] = k
		updateBody["key"] = k
	}

	_, diags := callAPI(ctx, r.client, pathL7RuleUpdate, updateBody)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	st, pollErr := r.pollRuleStatus(ctx, vpcID, ruleID, []string{"success", "active", "error"}, 15*time.Minute)
	if pollErr != nil || st == "error" {
		resp.Diagnostics.AddError(
			"Rule Update Failed",
			fmt.Sprintf("L7 Rule %d entered error state after update. Use 'terraform apply -replace=...' to recreate it.", ruleID),
		)
		return
	}

	r.readRule(ctx, vpcID, ruleID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *L7RuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state L7RuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueInt64()
	ruleID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || ruleID <= 0 {
		return
	}

	detail, gone, err := r.getRuleDetail(ctx, vpcID, ruleID)
	if gone || detail == nil {
		return
	}

	curStatus := strings.ToLower(detail.Status)
	if curStatus == "deleting" {
		// Parent policy is deleting or rule is already being deleted
		_ = r.pollRuleGone(ctx, vpcID, ruleID, 15*time.Minute)
		return
	}
	if curStatus == "pending" {
		_, _ = r.pollRuleStatus(ctx, vpcID, ruleID, []string{"success", "active", "error"}, 5*time.Minute)
	}

	listenerID := detail.VttLoadBalancerListenerId
	var lbID int64
	if listener, err := r.getListenerDetail(ctx, vpcID, listenerID); err == nil && listener != nil {
		lbID = listener.VttLoadBalancerId
	}

	unlock := LockLB(lbID)
	defer unlock()

	delBody := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerL7RuleId":   ruleID,
		"id":                        ruleID,
		"vttLoadBalancerL7PolicyId": detail.VttLoadBalancerL7PolicyId,
		"vttLoadBalancerListenerId": listenerID,
	}

	apiResp, diags := callAPI(ctx, r.client, pathL7RuleDelete, delBody)
	if isRuleNotFound(apiResp, diags) {
		return
	}
	if diags.HasError() {
		// If ERROR_L7_RULE_STATUS_NOT_ACTIVE, check if it transitioned to deleting
		if apiResp != nil && strings.Contains(apiResp.Message, "ERROR_L7_RULE_STATUS_NOT_ACTIVE") {
			latestDetail, isGone, _ := r.getRuleDetail(ctx, vpcID, ruleID)
			if isGone {
				return
			}
			if latestDetail != nil && strings.ToLower(latestDetail.Status) == "deleting" {
				_ = r.pollRuleGone(ctx, vpcID, ruleID, 15*time.Minute)
				return
			}
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	if err := r.pollRuleGone(ctx, vpcID, ruleID, 15*time.Minute); err != nil {
		resp.Diagnostics.AddError("Rule Delete Failed", err.Error())
		return
	}
}

func (r *L7RuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: <vpc_id>/<rule_id> or <vpc_id>/<policy_id>/<rule_id>
	parts := strings.Split(req.ID, "/")
	var vpcIDStr, ruleIDStr string
	var policyIDStr string

	if len(parts) == 2 {
		vpcIDStr = parts[0]
		ruleIDStr = parts[1]
	} else if len(parts) == 3 {
		vpcIDStr = parts[0]
		policyIDStr = parts[1]
		ruleIDStr = parts[2]
	} else {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			fmt.Sprintf("Expected '<vpc_id>/<rule_id>' or '<vpc_id>/<policy_id>/<rule_id>', got: %q", req.ID),
		)
		return
	}

	vpcID, err := strconv.ParseInt(vpcIDStr, 10, 64)
	if err != nil || vpcID <= 0 {
		resp.Diagnostics.AddError("Invalid VPC ID", fmt.Sprintf("VPC ID must be a positive integer, got: %q", vpcIDStr))
		return
	}

	ruleID, err := strconv.ParseInt(ruleIDStr, 10, 64)
	if err != nil || ruleID <= 0 {
		resp.Diagnostics.AddError("Invalid Rule ID", fmt.Sprintf("Rule ID must be a positive integer, got: %q", ruleIDStr))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vpc_id"), vpcID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(ruleID, 10))...)
	if policyIDStr != "" {
		policyID, pErr := strconv.ParseInt(policyIDStr, 10, 64)
		if pErr == nil && policyID > 0 {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_id"), policyID)...)
		}
	}
}

func (r *L7RuleResource) readRule(ctx context.Context, vpcID, ruleID int64, model *L7RuleResourceModel, diags *diag.Diagnostics) bool {
	detail, gone, err := r.getRuleDetail(ctx, vpcID, ruleID)
	if gone {
		return true
	}
	if err != nil {
		diags.AddError("Read Rule Failed", err.Error())
		return false
	}
	if detail == nil {
		return true
	}

	// If rule is deleting (e.g. parent policy deleted cascade), treat as gone
	if strings.ToLower(detail.Status) == "deleting" {
		return true
	}

	// Security / consistency check against wrong parent policy
	if !model.PolicyID.IsNull() && model.PolicyID.ValueInt64() > 0 {
		if detail.VttLoadBalancerL7PolicyId != model.PolicyID.ValueInt64() {
			tflog.Warn(ctx, fmt.Sprintf("Rule parent policy mismatch: got %d, expected %d", detail.VttLoadBalancerL7PolicyId, model.PolicyID.ValueInt64()))
			return true
		}
	} else {
		// Import scenario: populate policy_id from detail
		model.PolicyID = types.Int64Value(detail.VttLoadBalancerL7PolicyId)
	}

	model.ID = types.StringValue(strconv.FormatInt(detail.VttLoadBalancerL7RuleId, 10))
	model.ListenerID = types.Int64Value(detail.VttLoadBalancerListenerId)
	model.Type = types.StringValue(normalizeRuleType(detail.Type))
	model.CompareType = types.StringValue(normalizeCompareType(detail.CompareType))
	model.Value = types.StringValue(detail.Value)

	ruleType := normalizeRuleType(detail.Type)
	if ruleType == "HEADER" || ruleType == "COOKIE" {
		if detail.RuleKey != "" {
			model.Key = types.StringValue(detail.RuleKey)
		} else {
			model.Key = types.StringNull()
		}
	} else {
		model.Key = types.StringNull()
	}

	model.Status = types.StringValue(strings.ToLower(detail.Status))

	if strings.ToLower(detail.Status) == "error" {
		diags.AddWarning(
			"Rule In Error State",
			fmt.Sprintf("L7 Rule %d is in error state on backend. Use 'terraform apply -replace=...' to recreate it.", ruleID),
		)
	}

	return false
}

func (r *L7RuleResource) getRuleDetail(ctx context.Context, vpcID, ruleID int64) (*l7RuleDetailResponse, bool, error) {
	body := map[string]interface{}{
		"vpcId":                   vpcID,
		"vpc_id":                  vpcID,
		"customerId":              r.customerID,
		"customer_id":             r.customerID,
		"vttLoadBalancerL7RuleId": ruleID,
		"id":                      ruleID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathL7RuleDetail, body)
	if isRuleNotFound(apiResp, diags) {
		return nil, true, nil
	}
	if diags.HasError() {
		return nil, false, fmt.Errorf("call rule detail failed: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, true, nil
	}

	var detail l7RuleDetailResponse
	if err := json.Unmarshal(apiResp.Data, &detail); err != nil {
		return nil, false, fmt.Errorf("unmarshal rule detail: %w", err)
	}
	if detail.VttLoadBalancerL7RuleId == 0 && detail.ID != nil {
		detail.VttLoadBalancerL7RuleId = parseID(detail.ID)
	}
	return &detail, false, nil
}

func (r *L7RuleResource) pollRuleStatus(ctx context.Context, vpcID, ruleID int64, targetStatuses []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		detail, gone, err := r.getRuleDetail(ctx, vpcID, ruleID)
		if gone {
			return "gone", nil
		}
		if err == nil && detail != nil {
			st := strings.ToLower(strings.TrimSpace(detail.Status))
			if st == "active" {
				st = "success"
			}
			for _, target := range targetStatuses {
				if st == strings.ToLower(target) {
					return st, nil
				}
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("timeout waiting for L7 rule %d status (timeout=%s)", ruleID, timeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7RuleResource) pollRuleGone(ctx context.Context, vpcID, ruleID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		detail, gone, _ := r.getRuleDetail(ctx, vpcID, ruleID)
		if gone {
			return nil
		}
		if detail != nil && strings.ToLower(detail.Status) == "error" {
			return fmt.Errorf("L7 rule %d deletion failed on Octavia (status=error)", ruleID)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for L7 rule %d to disappear (timeout=%s)", ruleID, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7RuleResource) getParentPolicyDetail(ctx context.Context, vpcID, policyID int64) (*l7PolicyDetailResponse, bool, error) {
	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerL7PolicyId": policyID,
		"id":                        policyID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathL7PolicyDetail, body)
	if isPolicyNotFound(apiResp, diags) {
		return nil, true, nil
	}
	if diags.HasError() {
		return nil, false, fmt.Errorf("call policy detail: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, true, nil
	}
	var res l7PolicyDetailResponse
	if err := json.Unmarshal(apiResp.Data, &res); err != nil {
		return nil, false, fmt.Errorf("unmarshal policy detail: %w", err)
	}
	return &res, false, nil
}

func (r *L7RuleResource) waitPolicySuccess(ctx context.Context, vpcID, policyID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		p, gone, err := r.getParentPolicyDetail(ctx, vpcID, policyID)
		if err != nil {
			return err
		}
		if gone || p == nil {
			return fmt.Errorf("policy %d not found", policyID)
		}
		st := strings.ToLower(p.Status)
		if st == "success" || st == "active" {
			return nil
		}
		if st == "error" || st == "failed" {
			return fmt.Errorf("policy %d entered error state", policyID)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for policy %d to become success", policyID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7RuleResource) getListenerDetail(ctx context.Context, vpcID, listenerID int64) (*l7ListenerDetailResponse, error) {
	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerListenerId": listenerID,
		"id":                        listenerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathLoadBalancerListenerDetail, body)
	if diags.HasError() {
		return nil, fmt.Errorf("get listener detail: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, nil
	}
	var res l7ListenerDetailResponse
	if err := json.Unmarshal(apiResp.Data, &res); err != nil {
		return nil, fmt.Errorf("unmarshal listener detail: %w", err)
	}
	return &res, nil
}

func isRuleNotFound(apiResp *client.APIResponse, diags diag.Diagnostics) bool {
	if apiResp != nil {
		if strings.Contains(apiResp.Message, "ERROR_L7_RULE_NOT_FOUND") || isNotFoundMessage(apiResp.Message) {
			return true
		}
	}
	for _, d := range diags {
		if strings.Contains(d.Detail(), "ERROR_L7_RULE_NOT_FOUND") || strings.Contains(d.Summary(), "ERROR_L7_RULE_NOT_FOUND") {
			return true
		}
	}
	return false
}

func normalizeCompareType(s string) string {
	return strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
}

func normalizeRuleType(s string) string {
	return strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(s, " ", "_")))
}
