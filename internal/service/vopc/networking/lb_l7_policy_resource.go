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
	_ resource.Resource                   = (*L7PolicyResource)(nil)
	_ resource.ResourceWithConfigure      = (*L7PolicyResource)(nil)
	_ resource.ResourceWithImportState    = (*L7PolicyResource)(nil)
	_ resource.ResourceWithValidateConfig = (*L7PolicyResource)(nil)
)

var policyNameRegex = regexp.MustCompile(`^[a-zA-Z0-9.,@_\- ]+$`)

type L7PolicyResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7PolicyResource() resource.Resource {
	return &L7PolicyResource{}
}

type L7PolicyResourceModel struct {
	ID             types.String `tfsdk:"id"`
	VpcID          types.Int64  `tfsdk:"vpc_id"`
	ListenerID     types.Int64  `tfsdk:"listener_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Action         types.String `tfsdk:"action"`
	RedirectURL    types.String `tfsdk:"redirect_url"`
	RedirectPoolID types.Int64  `tfsdk:"redirect_pool_id"`
	Priority       types.Int64  `tfsdk:"priority"`
	Status         types.String `tfsdk:"status"`
	LoadBalancerID types.Int64  `tfsdk:"load_balancer_id"`
}

type l7PolicyDetailResponse struct {
	ID                        interface{} `json:"id"`
	VttLoadBalancerL7PolicyId int64       `json:"vttLoadBalancerL7PolicyId"`
	VttLoadBalancerListenerId int64       `json:"vttLoadBalancerListenerId"`
	Name                      string      `json:"name"`
	Description               string      `json:"description"`
	Action                    string      `json:"action"`
	RuleType                  string      `json:"ruleType"`
	RedirectURL               string      `json:"redirectUrl"`
	VttLoadBalancerPoolId     int64       `json:"vttLoadBalancerPoolId"`
	Priority                  *int64      `json:"priority"`
	Status                    string      `json:"status"`
	NumberOfRules             int64       `json:"numberOfRules"`
	PoolName                  string      `json:"poolName"`
}

type l7ListenerDetailResponse struct {
	VttLoadBalancerListenerId int64  `json:"vttLoadBalancerListenerId"`
	VttLoadBalancerId         int64  `json:"vttLoadBalancerId"`
	Name                      string `json:"name"`
	Protocol                  string `json:"protocol"`
	ProtocolPort              int64  `json:"protocolPort"`
	Status                    string `json:"status"`
}

type l7PoolDetailResponse struct {
	VttLoadBalancerPoolId int64  `json:"vttLoadBalancerPoolId"`
	VttLoadBalancerId     int64  `json:"vttLoadBalancerId"`
	Name                  string `json:"name"`
	Protocol              string `json:"protocol"`
	Status                string `json:"status"`
}

func (r *L7PolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_policy"
}

func (r *L7PolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ViettelIDC vLB Layer 7 Policy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The internal CMP ID of the L7 policy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the VPC owning the Load Balancer.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"listener_id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the Listener to which this L7 policy attaches.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the L7 policy (1-255 characters, regex: ^[a-zA-Z0-9.,@_\\- ]+$).",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
					stringvalidator.RegexMatches(policyNameRegex, "must contain only alphanumeric characters, spaces, and . , @ _ -"),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Description of the L7 policy (up to 255 characters).",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
				},
			},
			"action": schema.StringAttribute{
				Required:    true,
				Description: "Policy action: REJECT, REDIRECT_TO_URL, or REDIRECT_TO_POOL.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("REJECT", "REDIRECT_TO_URL", "REDIRECT_TO_POOL"),
				},
			},
			"redirect_url": schema.StringAttribute{
				Optional:    true,
				Description: "Destination URL when action is REDIRECT_TO_URL. Must start with http:// or https://, no spaces or control characters.",
			},
			"redirect_pool_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Destination Pool ID when action is REDIRECT_TO_POOL. Must belong to the same Load Balancer.",
			},
			"priority": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Evaluation priority (position) within the listener. Lower numbers are evaluated first.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Lifecycle status of the policy (pending, success, active, error).",
			},
			"load_balancer_id": schema.Int64Attribute{
				Computed:    true,
				Description: "ID of the Load Balancer derived from the listener.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *L7PolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

// ValidateConfig enforces Class A & B client-side validations (Section 9 of guide).
func (r *L7PolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg L7PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Action.IsUnknown() {
		return
	}

	action := cfg.Action.ValueString()

	switch action {
	case "REJECT":
		if !cfg.RedirectURL.IsNull() && !cfg.RedirectURL.IsUnknown() && cfg.RedirectURL.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_url"),
				"Invalid Attribute Combination",
				"redirect_url must not be set when action is REJECT",
			)
		}
		if !cfg.RedirectPoolID.IsNull() && !cfg.RedirectPoolID.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_pool_id"),
				"Invalid Attribute Combination",
				"redirect_pool_id must not be set when action is REJECT",
			)
		}
	case "REDIRECT_TO_URL":
		if cfg.RedirectURL.IsNull() || (!cfg.RedirectURL.IsUnknown() && strings.TrimSpace(cfg.RedirectURL.ValueString()) == "") {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_url"),
				"Missing Required Attribute",
				"redirect_url is required when action is REDIRECT_TO_URL",
			)
		} else if !cfg.RedirectURL.IsUnknown() {
			u := cfg.RedirectURL.ValueString()
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				resp.Diagnostics.AddAttributeError(
					path.Root("redirect_url"),
					"Invalid URL Format",
					"redirect_url must start with http:// or https:// (case-sensitive)",
				)
			}
			if strings.ContainsAny(u, " \t\r\n") {
				resp.Diagnostics.AddAttributeError(
					path.Root("redirect_url"),
					"Invalid URL Format",
					"redirect_url must not contain spaces or whitespace characters",
				)
			}
			for i := range u {
				if u[i] < 0x20 || u[i] == 0x7F {
					resp.Diagnostics.AddAttributeError(
						path.Root("redirect_url"),
						"Invalid URL Format",
						"redirect_url must not contain control characters",
					)
					break
				}
			}
			if len(u) > 1024 {
				resp.Diagnostics.AddAttributeError(
					path.Root("redirect_url"),
					"URL Too Long",
					"redirect_url length must not exceed 1024 characters",
				)
			}
		}
		if !cfg.RedirectPoolID.IsNull() && !cfg.RedirectPoolID.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_pool_id"),
				"Invalid Attribute Combination",
				"redirect_pool_id must not be set when action is REDIRECT_TO_URL",
			)
		}
	case "REDIRECT_TO_POOL":
		if cfg.RedirectPoolID.IsNull() || (!cfg.RedirectPoolID.IsUnknown() && cfg.RedirectPoolID.ValueInt64() <= 0) {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_pool_id"),
				"Missing Required Attribute",
				"redirect_pool_id is required and must be > 0 when action is REDIRECT_TO_POOL",
			)
		}
		if !cfg.RedirectURL.IsNull() && !cfg.RedirectURL.IsUnknown() && cfg.RedirectURL.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("redirect_url"),
				"Invalid Attribute Combination",
				"redirect_url must not be set when action is REDIRECT_TO_POOL",
			)
		}
	}

	if !cfg.Priority.IsNull() && !cfg.Priority.IsUnknown() {
		if cfg.Priority.ValueInt64() < 1 {
			resp.Diagnostics.AddAttributeError(
				path.Root("priority"),
				"Invalid Priority",
				"priority must be greater than or equal to 1",
			)
		}
	}
}

func (r *L7PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan L7PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	listenerID := plan.ListenerID.ValueInt64()
	action := plan.Action.ValueString()

	// 1. Fetch listener details and validate protocol + status
	listener, err := r.getListenerDetail(ctx, vpcID, listenerID)
	if err != nil {
		resp.Diagnostics.AddError("Listener Validation Error", err.Error())
		return
	}
	if listener == nil {
		resp.Diagnostics.AddError("Listener Not Found", fmt.Sprintf("Listener %d not found or does not belong to VPC %d", listenerID, vpcID))
		return
	}

	protoUpper := strings.ToUpper(listener.Protocol)
	if protoUpper != "HTTP" && protoUpper != "TERMINATED_HTTPS" {
		resp.Diagnostics.AddError("Incompatible Listener Protocol",
			fmt.Sprintf("Listener protocol %q is not supported for L7 policy; must be HTTP or TERMINATED_HTTPS", listener.Protocol))
		return
	}

	// Listener must be success/active (wait if pending, fail if error)
	listenerStatus := strings.ToLower(listener.Status)
	if listenerStatus == "error" {
		resp.Diagnostics.AddError("Listener In Error State", fmt.Sprintf("Listener %d is in error state", listenerID))
		return
	}
	if listenerStatus == "pending" {
		if err := r.waitListenerSuccess(ctx, vpcID, listenerID, 5*time.Minute); err != nil {
			resp.Diagnostics.AddError("Listener Pending Timeout", err.Error())
			return
		}
	}

	lbID := listener.VttLoadBalancerId
	plan.LoadBalancerID = types.Int64Value(lbID)

	// 2. Acquire Load Balancer mutex lock (Section 8.5)
	unlock := LockLB(lbID)
	defer unlock()

	// 3. Validate redirect_pool_id if action == REDIRECT_TO_POOL
	if action == "REDIRECT_TO_POOL" {
		poolID := plan.RedirectPoolID.ValueInt64()
		available, err := r.isPoolAvailable(ctx, vpcID, listenerID, poolID)
		if err != nil {
			resp.Diagnostics.AddError("Pool Validation Error", err.Error())
			return
		}
		if !available {
			resp.Diagnostics.AddError("Incompatible Pool",
				fmt.Sprintf("Pool %d is not available for listener %d (must belong to load balancer %d and have protocol HTTP, PROXY, or PROXYV2)", poolID, listenerID, lbID))
			return
		}

		pool, err := r.getPoolDetail(ctx, vpcID, lbID, poolID)
		if err != nil {
			resp.Diagnostics.AddError("Pool Detail Error", fmt.Sprintf("failed to get pool %d detail: %s", poolID, err))
			return
		}
		if pool == nil {
			resp.Diagnostics.AddError("Pool Not Found", fmt.Sprintf("Pool %d not found or does not belong to VPC %d", poolID, vpcID))
			return
		}
		poolStatus := strings.ToLower(pool.Status)
		if poolStatus == "error" {
			resp.Diagnostics.AddError("Pool In Error State", fmt.Sprintf("Pool %d is in error state", poolID))
			return
		}
		if poolStatus == "pending" {
			if err := r.waitPoolSuccess(ctx, vpcID, lbID, poolID, 5*time.Minute); err != nil {
				resp.Diagnostics.AddError("Pool Pending Timeout", err.Error())
				return
			}
		}
	}

	// 4. Attempt creation up to 3 times (Section 8.4)
	var createdID int64
	for attempt := 1; attempt <= 3; attempt++ {
		// NEVER send vttLoadBalancerL7RuleId in policy requests
		body := map[string]interface{}{
			"vpcId":                     vpcID,
			"vpc_id":                    vpcID,
			"customerId":                r.customerID,
			"customer_id":               r.customerID,
			"vttLoadBalancerListenerId": listenerID,
			"listener_id":               listenerID,
			"name":                      plan.Name.ValueString(),
			"action":                    action,
		}
		if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
			body["description"] = plan.Description.ValueString()
		}
		if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
			body["priority"] = plan.Priority.ValueInt64()
		}
		if action == "REDIRECT_TO_URL" {
			u := plan.RedirectURL.ValueString()
			body["redirectUrl"] = u
			body["redirect_url"] = u
		} else if action == "REDIRECT_TO_POOL" {
			p := plan.RedirectPoolID.ValueInt64()
			body["vttLoadBalancerPoolId"] = p
			body["redirect_pool_id"] = p
		}

		apiResp, diags := callAPI(ctx, r.client, pathL7PolicyCreate, body)
		if diags.HasError() {
			// Check if already created due to ambiguous network error
			if apiResp == nil {
				existingID, findErr := r.findPolicyInList(ctx, vpcID, listenerID, plan.Name.ValueString())
				if findErr == nil && existingID > 0 {
					createdID = existingID
					plan.ID = types.StringValue(strconv.FormatInt(createdID, 10))
					resp.State.Set(ctx, &plan)
					break
				}
			}
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
			resp.Diagnostics.AddError("Invalid Create Response", "API returned non-positive policy ID")
			return
		}

		// Save state immediately so resource is tracked if subsequent polling times out
		plan.ID = types.StringValue(strconv.FormatInt(createdID, 10))
		resp.State.Set(ctx, &plan)

		// Poll status until success, active, or error
		st, pollErr := r.pollPolicyStatus(ctx, vpcID, createdID, []string{"success", "active", "error"}, 15*time.Minute)
		if pollErr == nil && (st == "success" || st == "active") {
			break
		}

		// Polling saw error or failed
		tflog.Warn(ctx, fmt.Sprintf("L7 Policy %d attempt %d failed with status %s, cleaning up before retry", createdID, attempt, st))

		// Clean up failed policy
		delBody := map[string]interface{}{
			"vpcId":                     vpcID,
			"vpc_id":                    vpcID,
			"customerId":                r.customerID,
			"customer_id":               r.customerID,
			"vttLoadBalancerL7PolicyId": createdID,
			"id":                        createdID,
		}
		_, _ = callAPI(ctx, r.client, pathL7PolicyDelete, delBody)
		_ = r.pollPolicyGone(ctx, vpcID, createdID, 5*time.Minute)
		resp.State.RemoveResource(ctx)

		if attempt == 3 {
			resp.Diagnostics.AddError(
				"L7 Policy Creation Failed",
				fmt.Sprintf("Policy %d entered error state after 3 attempts (likely Octavia 409 conflict, listener not ready, or invalid parameter combination)", createdID),
			)
			return
		}

		time.Sleep(time.Duration(attempt*5) * time.Second)
	}

	// 5. Read back final state
	r.readPolicy(ctx, vpcID, createdID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *L7PolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state L7PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueInt64()
	policyID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || policyID <= 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	gone := r.readPolicy(ctx, vpcID, policyID, &state, &resp.Diagnostics)
	if gone {
		resp.State.RemoveResource(ctx)
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *L7PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan L7PolicyResourceModel
	var state L7PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := plan.VpcID.ValueInt64()
	policyID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || policyID <= 0 {
		resp.Diagnostics.AddError("Invalid Policy ID", "Current state has invalid policy ID")
		return
	}

	detail, gone, err := r.getPolicyDetail(ctx, vpcID, policyID)
	if gone || detail == nil {
		resp.Diagnostics.AddError("Policy Not Found", fmt.Sprintf("L7 Policy %d not found on backend", policyID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Policy Detail Read Error", err.Error())
		return
	}

	currentStatus := strings.ToLower(detail.Status)
	if currentStatus == "error" {
		resp.Diagnostics.AddError(
			"Cannot Update Policy in Error State",
			fmt.Sprintf("L7 Policy %d is in error state. Terraform cannot update it; use 'terraform apply -replace=...' to recreate it.", policyID),
		)
		return
	}
	if currentStatus == "pending" {
		st, pollErr := r.pollPolicyStatus(ctx, vpcID, policyID, []string{"success", "active", "error"}, 10*time.Minute)
		if pollErr != nil || st == "error" {
			resp.Diagnostics.AddError("Policy Update Precondition Failed", fmt.Sprintf("Policy %d did not settle to success before update (status=%s)", policyID, st))
			return
		}
	}

	listenerID := detail.VttLoadBalancerListenerId
	listener, err := r.getListenerDetail(ctx, vpcID, listenerID)
	if err != nil || listener == nil {
		resp.Diagnostics.AddError("Listener Validation Error", fmt.Sprintf("Listener %d could not be fetched", listenerID))
		return
	}
	lbID := listener.VttLoadBalancerId

	unlock := LockLB(lbID)
	defer unlock()

	// Send FULL desired state in update body (Section 4.5 & 8.4)
	// Backend does not auto-populate redirectUrl/vttLoadBalancerPoolId on update
	updateBody := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerL7PolicyId": policyID,
		"id":                        policyID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
		"name":                      plan.Name.ValueString(),
		"action":                    plan.Action.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		updateBody["description"] = plan.Description.ValueString()
	} else {
		updateBody["description"] = ""
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		updateBody["priority"] = plan.Priority.ValueInt64()
	}
	if plan.Action.ValueString() == "REDIRECT_TO_URL" {
		u := plan.RedirectURL.ValueString()
		updateBody["redirectUrl"] = u
		updateBody["redirect_url"] = u
	} else if plan.Action.ValueString() == "REDIRECT_TO_POOL" {
		p := plan.RedirectPoolID.ValueInt64()
		updateBody["vttLoadBalancerPoolId"] = p
		updateBody["redirect_pool_id"] = p
	}

	_, diags := callAPI(ctx, r.client, pathL7PolicyUpdate, updateBody)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	st, pollErr := r.pollPolicyStatus(ctx, vpcID, policyID, []string{"success", "active", "error"}, 15*time.Minute)
	if pollErr != nil || st == "error" {
		resp.Diagnostics.AddError(
			"Policy Update Failed",
			fmt.Sprintf("L7 Policy %d entered error state after update. Use 'terraform apply -replace=...' to recreate it.", policyID),
		)
		return
	}

	r.readPolicy(ctx, vpcID, policyID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *L7PolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state L7PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueInt64()
	policyID, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil || policyID <= 0 {
		return
	}

	detail, gone, err := r.getPolicyDetail(ctx, vpcID, policyID)
	if gone || detail == nil {
		return // already deleted
	}

	curStatus := strings.ToLower(detail.Status)
	if curStatus == "pending" {
		_, _ = r.pollPolicyStatus(ctx, vpcID, policyID, []string{"success", "active", "error"}, 5*time.Minute)
	}

	if curStatus != "deleting" {
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
			"vttLoadBalancerL7PolicyId": policyID,
			"id":                        policyID,
		}
		apiResp, diags := callAPI(ctx, r.client, pathL7PolicyDelete, delBody)
		if isPolicyNotFound(apiResp, diags) {
			return
		}
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		if err := r.pollPolicyGone(ctx, vpcID, policyID, 15*time.Minute); err != nil {
			resp.Diagnostics.AddError("Policy Delete Failed", err.Error())
			return
		}
	} else {
		if err := r.pollPolicyGone(ctx, vpcID, policyID, 15*time.Minute); err != nil {
			resp.Diagnostics.AddError("Policy Delete Failed", err.Error())
			return
		}
	}
}

func (r *L7PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: <vpc_id>/<policy_id>
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID Format",
			fmt.Sprintf("Expected '<vpc_id>/<policy_id>', got: %q", req.ID),
		)
		return
	}

	vpcID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || vpcID <= 0 {
		resp.Diagnostics.AddError("Invalid VPC ID", fmt.Sprintf("VPC ID must be a positive integer, got: %q", parts[0]))
		return
	}

	policyID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || policyID <= 0 {
		resp.Diagnostics.AddError("Invalid Policy ID", fmt.Sprintf("Policy ID must be a positive integer, got: %q", parts[1]))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vpc_id"), vpcID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(policyID, 10))...)
}

// readPolicy populates the model from backend detail endpoint.
// Returns true if the resource is gone.
func (r *L7PolicyResource) readPolicy(ctx context.Context, vpcID, policyID int64, model *L7PolicyResourceModel, diags *diag.Diagnostics) bool {
	detail, gone, err := r.getPolicyDetail(ctx, vpcID, policyID)
	if gone {
		return true
	}
	if err != nil {
		diags.AddError("Read Policy Failed", err.Error())
		return false
	}
	if detail == nil {
		return true
	}

	// Security check against wrong VPC/listener mapping
	if !model.ListenerID.IsNull() && model.ListenerID.ValueInt64() > 0 {
		if detail.VttLoadBalancerListenerId != model.ListenerID.ValueInt64() {
			tflog.Warn(ctx, fmt.Sprintf("Policy listener mismatch: got %d, expected %d", detail.VttLoadBalancerListenerId, model.ListenerID.ValueInt64()))
			return true
		}
	} else {
		// Import scenario: verify listener belongs to VPC (Section 10 #7)
		listener, lErr := r.getListenerDetail(ctx, vpcID, detail.VttLoadBalancerListenerId)
		if lErr != nil || listener == nil {
			diags.AddError("Policy Security Verification Failed",
				fmt.Sprintf("Listener %d for policy %d does not exist in VPC %d", detail.VttLoadBalancerListenerId, policyID, vpcID))
			return true
		}
		model.ListenerID = types.Int64Value(detail.VttLoadBalancerListenerId)
		model.LoadBalancerID = types.Int64Value(listener.VttLoadBalancerId)
	}

	model.ID = types.StringValue(strconv.FormatInt(detail.VttLoadBalancerL7PolicyId, 10))
	model.Name = types.StringValue(detail.Name)
	model.Action = types.StringValue(normalizeAction(detail.Action))

	if detail.Description != "" {
		model.Description = types.StringValue(detail.Description)
	} else if !model.Description.IsNull() && model.Description.ValueString() == "" {
		model.Description = types.StringValue("")
	} else {
		model.Description = types.StringNull()
	}

	if detail.RedirectURL != "" {
		model.RedirectURL = types.StringValue(detail.RedirectURL)
	} else {
		model.RedirectURL = types.StringNull()
	}

	if detail.VttLoadBalancerPoolId > 0 {
		model.RedirectPoolID = types.Int64Value(detail.VttLoadBalancerPoolId)
	} else {
		model.RedirectPoolID = types.Int64Null()
	}

	if detail.Priority != nil {
		model.Priority = types.Int64Value(*detail.Priority)
	}

	model.Status = types.StringValue(strings.ToLower(detail.Status))

	if strings.ToLower(detail.Status) == "error" {
		diags.AddWarning(
			"Policy In Error State",
			fmt.Sprintf("L7 Policy %d is in error state on backend. Use 'terraform apply -replace=...' to recreate it.", policyID),
		)
	}

	// Fill LoadBalancerID if not set
	if model.LoadBalancerID.IsNull() || model.LoadBalancerID.ValueInt64() == 0 {
		if listener, err := r.getListenerDetail(ctx, vpcID, detail.VttLoadBalancerListenerId); err == nil && listener != nil {
			model.LoadBalancerID = types.Int64Value(listener.VttLoadBalancerId)
		}
	}

	return false
}

func (r *L7PolicyResource) getPolicyDetail(ctx context.Context, vpcID, policyID int64) (*l7PolicyDetailResponse, bool, error) {
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
		return nil, false, fmt.Errorf("call detail failed: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, true, nil
	}

	var detail l7PolicyDetailResponse
	if err := json.Unmarshal(apiResp.Data, &detail); err != nil {
		return nil, false, fmt.Errorf("unmarshal policy detail: %w", err)
	}
	if detail.VttLoadBalancerL7PolicyId == 0 && detail.ID != nil {
		detail.VttLoadBalancerL7PolicyId = parseID(detail.ID)
	}
	return &detail, false, nil
}

func (r *L7PolicyResource) pollPolicyStatus(ctx context.Context, vpcID, policyID int64, targetStatuses []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		detail, gone, err := r.getPolicyDetail(ctx, vpcID, policyID)
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
			return "", fmt.Errorf("timeout waiting for L7 policy %d status (timeout=%s)", policyID, timeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7PolicyResource) pollPolicyGone(ctx context.Context, vpcID, policyID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		detail, gone, _ := r.getPolicyDetail(ctx, vpcID, policyID)
		if gone {
			return nil
		}
		if detail != nil && strings.ToLower(detail.Status) == "error" {
			return fmt.Errorf("L7 policy %d deletion failed on Octavia (status=error)", policyID)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for L7 policy %d to disappear (timeout=%s)", policyID, timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7PolicyResource) getListenerDetail(ctx context.Context, vpcID, listenerID int64) (*l7ListenerDetailResponse, error) {
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

func (r *L7PolicyResource) waitListenerSuccess(ctx context.Context, vpcID, listenerID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		l, err := r.getListenerDetail(ctx, vpcID, listenerID)
		if err != nil {
			return err
		}
		if l == nil {
			return fmt.Errorf("listener %d not found", listenerID)
		}
		st := strings.ToLower(l.Status)
		if st == "success" || st == "active" {
			return nil
		}
		if st == "error" || st == "failed" {
			return fmt.Errorf("listener %d entered error state", listenerID)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for listener %d to become success", listenerID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7PolicyResource) isPoolAvailable(ctx context.Context, vpcID, listenerID, poolID int64) (bool, error) {
	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathL7PolicyPools, body)
	if diags.HasError() {
		return false, fmt.Errorf("list available pools: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return false, nil
	}
	var pools []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(apiResp.Data, &pools); err != nil {
		return false, fmt.Errorf("unmarshal available pools: %w", err)
	}
	for _, p := range pools {
		if p.ID == poolID {
			return true, nil
		}
	}
	return false, nil
}

func (r *L7PolicyResource) getPoolDetail(ctx context.Context, vpcID, lbID, poolID int64) (*l7PoolDetailResponse, error) {
	body := map[string]interface{}{
		"vpcId":                 vpcID,
		"vpc_id":                vpcID,
		"customerId":            r.customerID,
		"customer_id":           r.customerID,
		"vttLoadBalancerPoolId": poolID,
		"id":                    poolID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathLoadBalancerPoolDetail, body)
	if !diags.HasError() && apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		var res l7PoolDetailResponse
		if err := json.Unmarshal(apiResp.Data, &res); err == nil && (res.VttLoadBalancerPoolId != 0 || res.Status != "") {
			return &res, nil
		}
	}

	// Fallback to pathLoadBalancerPools (/csa/api/v1/networking/loadbalancer/pool-by-lb/all)
	if lbID > 0 {
		lbBody := map[string]interface{}{
			"vpcId":             vpcID,
			"customerId":        r.customerID,
			"vttLoadBalancerId": lbID,
		}
		lbResp, lbDiags := callAPI(ctx, r.client, pathLoadBalancerPools, lbBody)
		if !lbDiags.HasError() && lbResp != nil && len(lbResp.Data) > 0 && string(lbResp.Data) != "null" {
			var pools []l7PoolDetailResponse
			if err := json.Unmarshal(lbResp.Data, &pools); err == nil {
				for _, p := range pools {
					if p.VttLoadBalancerPoolId == poolID {
						return &p, nil
					}
				}
			}
		}
	}
	return nil, nil
}

func (r *L7PolicyResource) waitPoolSuccess(ctx context.Context, vpcID, lbID, poolID int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		p, err := r.getPoolDetail(ctx, vpcID, lbID, poolID)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("pool %d not found", poolID)
		}
		st := strings.ToLower(p.Status)
		if st == "success" || st == "active" {
			return nil
		}
		if st == "error" || st == "failed" {
			return fmt.Errorf("pool %d entered error state", poolID)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for pool %d to become success", poolID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (r *L7PolicyResource) findPolicyInList(ctx context.Context, vpcID, listenerID int64, name string) (int64, error) {
	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                r.customerID,
		"customer_id":               r.customerID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
		"pageIndex":                 0,
		"pageSize":                  1000,
	}
	apiResp, diags := callAPI(ctx, r.client, pathL7PolicyList, body)
	if diags.HasError() {
		return 0, fmt.Errorf("find policy in list: %s", diags[0].Detail())
	}
	if apiResp == nil || len(apiResp.Data) == 0 {
		return 0, nil
	}
	var listResp struct {
		Items []l7PolicyDetailResponse `json:"items"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		return 0, err
	}
	for _, item := range listResp.Items {
		if item.Name == name {
			if item.VttLoadBalancerL7PolicyId > 0 {
				return item.VttLoadBalancerL7PolicyId, nil
			}
			return parseID(item.ID), nil
		}
	}
	return 0, nil
}

func isPolicyNotFound(apiResp *client.APIResponse, diags diag.Diagnostics) bool {
	if apiResp != nil {
		if strings.Contains(apiResp.Message, "ERROR_L7_POLICY_NOT_FOUND") || isNotFoundMessage(apiResp.Message) {
			return true
		}
	}
	for _, d := range diags {
		if strings.Contains(d.Detail(), "ERROR_L7_POLICY_NOT_FOUND") || strings.Contains(d.Summary(), "ERROR_L7_POLICY_NOT_FOUND") {
			return true
		}
	}
	return false
}

func normalizeAction(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.ReplaceAll(s, " ", "_")
}

func parseID(val interface{}) int64 {
	switch v := val.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}
