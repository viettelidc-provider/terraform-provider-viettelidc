package networking

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/iac/client"
)

var (
	_ resource.Resource                = (*SecurityGroupRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*SecurityGroupRuleResource)(nil)
	_ resource.ResourceWithImportState = (*SecurityGroupRuleResource)(nil)
)

// sgRuleLocks serialises concurrent rule operations on the same security group.
// The API rejects a second rule mutation while the first is still in progress.
var sgRuleLocks sync.Map // map[sgID string]*sync.Mutex

// SecurityGroupRuleResource implements `viettelidc_security_group_rule`.
//
// The API does NOT have a delete endpoint for rules; deletion is done by
// calling the update endpoint with action="Delete". There is also no single-rule
// detail endpoint, so Read is implemented via the inbound/outbound list.
//
// Import ID format: "<security_group_id>/<rule_id>"
type SecurityGroupRuleResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type SecurityGroupRuleResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	Direction       types.String `tfsdk:"direction"`
	RuleType        types.String `tfsdk:"rule_type"`
	ProtocolName    types.String `tfsdk:"protocol_name"`
	Port            types.String `tfsdk:"port"`
	SourceIP        types.String `tfsdk:"source_ip"`
	DestinationIP   types.String `tfsdk:"destination_ip"`
	Action          types.String `tfsdk:"action"`
	IsValid         types.Bool   `tfsdk:"is_valid"`
	VpcID           types.String `tfsdk:"vpc_id"`
}

func NewSecurityGroupRuleResource() resource.Resource { return &SecurityGroupRuleResource{} }

func (r *SecurityGroupRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_security_group_rule"
}

func (r *SecurityGroupRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Security Group Rule — inbound or outbound rule attached to a Security Group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"security_group_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"direction": schema.StringAttribute{
				Required:    true,
				Description: `Direction of traffic: "in" (inbound/ingress) or "out" (outbound/egress).`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule_type": schema.StringAttribute{
				Required:    true,
				Description: "Rule type as shown in portal: \"Custom TCP\", \"Custom UDP\", \"All TCP\", \"All UDP\", \"All ICMP - IPv4\", \"SSH\", \"DNS\", \"HTTP\", \"HTTPS\", \"IMAP\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"protocol_name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_ip": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"destination_ip": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"action": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: `Default "New". Set to "Delete" to remove.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_valid": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SecurityGroupRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *SecurityGroupRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Serialize per-SG: API only allows one rule mutation at a time.
	sgMuRaw, _ := sgRuleLocks.LoadOrStore(plan.SecurityGroupID.ValueString(), &sync.Mutex{})
	sgMu := sgMuRaw.(*sync.Mutex)
	sgMu.Lock()
	defer sgMu.Unlock()

	body := r.buildRuleBody(&plan, vpcID, "New")
	apiResp, diags := callAPI(ctx, r.client, pathSGRuleCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ruleID, err := extractSGRuleID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("SG Rule create: missing id", err.Error())
		return
	}

	plan.ID = types.StringValue(ruleID)
	plan.VpcID = types.StringValue(vpcID)
	plan.IsValid = types.BoolValue(true)

	// SG rule list endpoint only returns rules with status "success";
	// newly-created rules may still be in "pending" state. Poll until visible.
	deadline := time.Now().Add(2 * time.Minute)
	var found bool
	for {
		var pollDiags diag.Diagnostics
		found = r.populateFromList(ctx, &plan, &pollDiags)
		if found && !pollDiags.HasError() {
			break
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError(
				"SG Rule did not become visible",
				fmt.Sprintf("SG Rule %s did not appear in the list within 2 minutes", ruleID),
			)
			return
		}
		time.Sleep(3 * time.Second)
	}
	// Resolve any remaining Unknown computed fields to empty defaults.
	if plan.DestinationIP.IsUnknown() {
		plan.DestinationIP = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecurityGroupRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found := r.populateFromList(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: all meaningful attributes have RequiresReplace.
func (r *SecurityGroupRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *SecurityGroupRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := state.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	// Serialize per-SG: API only allows one rule mutation at a time.
	sgMuRaw, _ := sgRuleLocks.LoadOrStore(state.SecurityGroupID.ValueString(), &sync.Mutex{})
	sgMu := sgMuRaw.(*sync.Mutex)
	sgMu.Lock()
	defer sgMu.Unlock()

	body := r.buildRuleBody(&state, vpcID, "Delete")

	// Simple retry in case a transient lock is still releasing on the backend.
	// If a parallel delete is in progress, the API returns ERROR_SECURITY_RULE_IS_IN_ANOTHER_PROCESS.
	// Wait and retry for up to 2 minutes.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		apiResp, diags := callAPI(ctx, r.client, pathSGRuleUpdate, body)
		if !diags.HasError() {
			// API returned 200 — rule deleted.
			return
		}
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return // idempotent
		}
		// Retry on concurrent-process conflict.
		if apiResp != nil && strings.Contains(apiResp.Message, "ANOTHER_PROCESS") {
			if time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
		}
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *SecurityGroupRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import format: "<security_group_id>/<rule_id>"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected format: <security_group_id>/<rule_id>",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("security_group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// ---------- Helpers ----------

// normalizeDirection converts user-facing direction values to the API-expected "in"/"out".
// Accepts: "in", "out", "Inbound", "Outbound", "ingress", "egress" (case-insensitive).
func normalizeDirection(d string) string {
	switch strings.ToLower(d) {
	case "inbound", "ingress":
		return "in"
	case "outbound", "egress":
		return "out"
	default:
		return strings.ToLower(d) // "in" / "out" pass through
	}
}

// deriveProtocolName extracts the protocol from a rule type string.
// "Custom TCP" → "TCP", "All UDP" → "UDP", "All ICMP - IPv4" → "ICMP", etc.
func deriveProtocolName(ruleType string) string {
	upper := strings.ToUpper(ruleType)
	for _, proto := range []string{"TCP", "UDP", "ICMP"} {
		if strings.Contains(upper, proto) {
			return proto
		}
	}
	return ""
}

// buildRuleBody constructs the request body for create or delete (update).
func (r *SecurityGroupRuleResource) buildRuleBody(m *SecurityGroupRuleResourceModel, vpcID, action string) map[string]interface{} {
	dir := normalizeDirection(m.Direction.ValueString())

	body := map[string]interface{}{
		"security_group_id": m.SecurityGroupID.ValueString(),
		"direction":         dir,
		"rule_type":         m.RuleType.ValueString(),
		"action":            action,
		"is_valid":          true,
		"totalRuleAdd":      1,
		"vpc_id":            vpcID,
		"customer_id":       r.customerID,
	}

	// Derive protocol_name from rule_type when not explicitly set.
	// "Custom TCP" → protocol_name=TCP, "All UDP" → protocol_name=UDP, etc.
	protocolName := m.ProtocolName.ValueString()
	if protocolName == "" {
		protocolName = deriveProtocolName(m.RuleType.ValueString())
	}
	if protocolName != "" {
		body["protocol_name"] = protocolName
	}

	if v := m.Port.ValueString(); v != "" {
		body["port"] = v
	}

	// source / destination fields required by the API.
	if dir == "in" {
		if v := m.SourceIP.ValueString(); v != "" {
			body["source_ip"] = v
			if v == "0.0.0.0/0" {
				body["source"] = "anywhere"
			} else {
				body["source"] = "custom"
			}
		}
		body["destination"] = "custom"
	} else {
		if v := m.DestinationIP.ValueString(); v != "" {
			body["destination_ip"] = v
			if v == "0.0.0.0/0" {
				body["destination"] = "anywhere"
			} else {
				body["destination"] = "custom"
			}
		}
		body["source"] = "custom"
	}

	if !m.ID.IsNull() && !m.ID.IsUnknown() && m.ID.ValueString() != "" {
		body["id"] = m.ID.ValueString()
		// The update endpoint requires vttSecurityRuleId as an integer.
		if n, err := strconv.Atoi(m.ID.ValueString()); err == nil {
			body["vttSecurityRuleId"] = n
		}
	}
	return body
}

// populateFromList fetches the SG detail and finds the matching rule in the embedded rules list.
// The inbound/outbound list endpoints only return rules with status "success", so they miss
// newly-created rules still in "pending" state. The SG detail endpoint returns all rules.
// Returns true if found.
func (r *SecurityGroupRuleResource) populateFromList(ctx context.Context, m *SecurityGroupRuleResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	// Use the direction-specific rule list endpoint (inbound/outbound list)
	// instead of pathSGDetail which does not reliably return individual rule status.
	listPath := pathSGRuleInboundList
	if normalizeDirection(m.Direction.ValueString()) == "out" {
		listPath = pathSGRuleOutboundList
	}
	body := map[string]interface{}{
		"pageIndex":          0,
		"pageSize":           9999,
		"filters":            []interface{}{},
		"vttSecurityGroupId": m.SecurityGroupID.ValueString(),
		"vpcId":              vpcID,
		"customerId":         r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, listPath, body)
	if d.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return false
		}
		diags.Append(d...)
		return true
	}
	var listData map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err != nil {
		diags.AddError("decode SG rule list", err.Error())
		return true
	}
	// Propagate vpc_id when not already known.
	if m.VpcID.IsNull() || m.VpcID.IsUnknown() || m.VpcID.ValueString() == "" {
		m.VpcID = types.StringValue(vpcID)
	}
	itemsRaw, _ := listData["items"].([]interface{})
	targetID := m.ID.ValueString()
	for _, ruleRaw := range itemsRaw {
		item, ok := ruleRaw.(map[string]interface{})
		if !ok {
			continue
		}
		itemID := asIDString(item, "id")
		if itemID == "" {
			itemID = asIDString(item, "vttSecurityRuleId")
		}
		if itemID == targetID {
			// Treat rule as gone if the backend marks it as deleted/deleting.
			st := strings.ToUpper(asString(item, "status"))
			if st == "DELETED" || st == "DELETING" {
				return false
			}
			mapSGRuleResponse(item, m)
			return true
		}
	}
	return false
}

func extractSGRuleID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("id not found in response: %s", string(resp.Data))
}

func decodeSGRuleList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	var raw interface{}
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	if arr, ok := raw.([]interface{}); ok {
		return toMapSlice(arr), nil
	}
	if m, ok := raw.(map[string]interface{}); ok {
		for _, key := range []string{"items", "content", "data"} {
			if list, ok := m[key].([]interface{}); ok {
				return toMapSlice(list), nil
			}
		}
	}
	return nil, fmt.Errorf("unexpected list structure: %T", raw)
}

func mapSGRuleResponse(item map[string]interface{}, m *SecurityGroupRuleResourceModel) {
	if v := asIDString(item, "id"); v != "" {
		m.ID = types.StringValue(v)
	}
	if v := asString(item, "direction"); v != "" {
		m.Direction = types.StringValue(normalizeDirection(v))
	}
	if v := asString(item, "type"); v != "" {
		m.RuleType = types.StringValue(v)
	}
	if v := asString(item, "protocolName"); v != "" {
		m.ProtocolName = types.StringValue(v)
	}
	if v := asString(item, "port"); v != "" {
		m.Port = types.StringValue(v)
	}
	if v := asString(item, "sourceIP"); v != "" {
		m.SourceIP = types.StringValue(v)
	}
	// Always resolve destination_ip to avoid Unknown after apply.
	if v := asString(item, "destinationIP"); v != "" {
		m.DestinationIP = types.StringValue(v)
	} else {
		m.DestinationIP = types.StringValue("")
	}
	// The API action field is "New"/"Delete" (operational), not Allow/Deny.
	// Default to "Allow" so the TF attribute doesn't show as computed drift after import/read.
	if m.Action.IsNull() || m.Action.IsUnknown() {
		m.Action = types.StringValue("Allow")
	}
	if v, ok := item["isValid"].(bool); ok {
		m.IsValid = types.BoolValue(v)
	}
}
