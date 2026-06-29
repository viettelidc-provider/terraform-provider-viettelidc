// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = (*AutoscaleGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*AutoscaleGroupResource)(nil)
	_ resource.ResourceWithImportState = (*AutoscaleGroupResource)(nil)
)

// AutoscaleGroupResource implements `viettelidc_autoscale_group`.
type AutoscaleGroupResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

// AutoscaleGroupResourceModel mirrors the resource schema.
type AutoscaleGroupResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	LaunchTemplateID  types.String `tfsdk:"launch_template_id"`
	IsAutoscale       types.Bool   `tfsdk:"is_autoscale"`
	DesiredCapacity   types.Int64  `tfsdk:"desired_capacity"`
	MinSize           types.Int64  `tfsdk:"min_size"`
	MaxSize           types.Int64  `tfsdk:"max_size"`
	MetricType        types.String `tfsdk:"metric_type"`
	ScaleOutThreshold types.Int64  `tfsdk:"scale_out_threshold"`
	ScaleInThreshold  types.Int64  `tfsdk:"scale_in_threshold"`
	HasLoadBalancer   types.Bool   `tfsdk:"has_load_balancer"`
	VpcID             types.String `tfsdk:"vpc_id"`
}

// NewAutoscaleGroupResource constructs the resource (registered in iac/provider.go).
func NewAutoscaleGroupResource() resource.Resource { return &AutoscaleGroupResource{} }

func (r *AutoscaleGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_autoscale_group"
}

func (r *AutoscaleGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Autoscale Group. All attributes are immutable (RequiresReplace / ForceNew). " +
			"Read() uses list+filter because the API has no detail endpoint for ASG.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Autoscale Group ID assigned by the system.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Autoscale Group name. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"launch_template_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the Launch Template to use. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"is_autoscale": schema.BoolAttribute{
				Required:    true,
				Description: "Whether automatic scaling is enabled. Immutable.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"desired_capacity": schema.Int64Attribute{
				Required:    true,
				Description: "Desired number of instances. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"min_size": schema.Int64Attribute{
				Required:    true,
				Description: "Minimum number of instances. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"max_size": schema.Int64Attribute{
				Required:    true,
				Description: "Maximum number of instances. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"metric_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: `Scaling metric type (default: "CPU"). Immutable.`,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scale_out_threshold": schema.Int64Attribute{
				Required:    true,
				Description: "CPU % threshold to scale out. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"scale_in_threshold": schema.Int64Attribute{
				Required:    true,
				Description: "CPU % threshold to scale in. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"has_load_balancer": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the ASG is attached to a Load Balancer (default: false). Immutable.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Falls back to the provider default vpc_id when unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *AutoscaleGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *AutoscaleGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AutoscaleGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, diags := resolveVpcID(plan.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := buildAutoscaleGroupCreateBody(plan, r.customerID, vpcID)
	apiResp, diags := callAPI(ctx, r.client, pathAutoscaleGroupCreate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := extractAutoscaleGroupID(apiResp)
	if err != nil {
		resp.Diagnostics.AddError("Create response missing autoscale group ID", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	plan.VpcID = types.StringValue(vpcID)
	if plan.MetricType.IsNull() || plan.MetricType.IsUnknown() {
		plan.MetricType = types.StringValue("CPU")
	}

	// ASG creation is async: the group may not appear in the list immediately.
	// Poll until readInto finds it (desired_capacity > 0 means VMs are spinning up).
	deadline := time.Now().Add(5 * time.Minute)
	for {
		var pollDiags diag.Diagnostics
		if r.readInto(ctx, &plan, &pollDiags) {
			break
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError(
				"AutoscaleGroup did not become visible",
				fmt.Sprintf("AutoscaleGroup %s did not appear in the list within 5 minutes", id),
			)
			return
		}
		time.Sleep(5 * time.Second)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AutoscaleGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AutoscaleGroupResourceModel
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

// Update is not implemented — all attributes are RequiresReplace (ForceNew).
func (r *AutoscaleGroupResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *AutoscaleGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AutoscaleGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID, vpcDiags := resolveVpcID(state.VpcID.ValueString(), r.defaultVpcID)
	resp.Diagnostics.Append(vpcDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, diags := callAPI(ctx, r.client, pathAutoscaleGroupDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return // Already deleted — idempotent.
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until ASG is gone from the list (delete is async — scales down VMs first).
	deadline := time.Now().Add(10 * time.Minute)
	for {
		var tmpDiags diag.Diagnostics
		tmpState := state
		if !r.readInto(ctx, &tmpState, &tmpDiags) {
			break // ASG no longer in list — deletion complete.
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError(
				"AutoscaleGroup did not disappear after delete",
				fmt.Sprintf("AutoscaleGroup %s still present in list after 10 minutes", state.ID.ValueString()),
			)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (r *AutoscaleGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto implements Decision 8 (list+filter): fetches the ASG list and
// finds the entry matching state.ID. Returns false (drift) when not found.
func (r *AutoscaleGroupResource) readInto(ctx context.Context, m *AutoscaleGroupResourceModel, diags *diag.Diagnostics) bool {
	vpcID := m.VpcID.ValueString()
	if vpcID == "" {
		vpcID = r.defaultVpcID
	}

	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	apiResp, d := callAPI(ctx, r.client, pathAutoscaleGroupList, body)
	if d.HasError() {
		diags.Append(d...)
		return true
	}

	items, err := decodeAutoscaleGroupList(apiResp)
	if err != nil {
		diags.AddError("Decode autoscale group list", err.Error())
		return true
	}

	targetID := m.ID.ValueString()
	for _, raw := range items {
		if asIDString(raw, "id") == targetID {
			mapAutoscaleGroupResponse(raw, m, vpcID)
			return true
		}
	}
	// Not found in list — treat as drift.
	return false
}

// ---------- Pure helpers (unit-tested) ----------

// buildAutoscaleGroupCreateBody constructs the POST body.
func buildAutoscaleGroupCreateBody(plan AutoscaleGroupResourceModel, customerID, vpcID string) map[string]interface{} {
	// launch_template_id is stored as string in state but the API expects an integer.
	ltID := plan.LaunchTemplateID.ValueString()
	var ltIDVal interface{} = ltID
	if n, err := strconv.ParseInt(ltID, 10, 64); err == nil {
		ltIDVal = n
	}
	body := map[string]interface{}{
		"name":                plan.Name.ValueString(),
		"launch_template_id":  ltIDVal,
		"is_autoscale":        plan.IsAutoscale.ValueBool(),
		"desired_capacity":    plan.DesiredCapacity.ValueInt64(),
		"min_size":            plan.MinSize.ValueInt64(),
		"max_size":            plan.MaxSize.ValueInt64(),
		"scale_out_threshold": plan.ScaleOutThreshold.ValueInt64(),
		"scale_in_threshold":  plan.ScaleInThreshold.ValueInt64(),
		"vpc_id":              vpcID,
		"customer_id":         customerID,
	}
	// metric_type defaults to "CPU" when not specified.
	metricType := plan.MetricType.ValueString()
	if metricType == "" {
		metricType = "CPU"
	}
	body["metric_type"] = metricType
	// Only include has_load_balancer when explicitly configured —
	// avoids silently hard-coding false when the user omits the attribute.
	if !plan.HasLoadBalancer.IsNull() && !plan.HasLoadBalancer.IsUnknown() {
		body["has_load_balancer"] = plan.HasLoadBalancer.ValueBool()
	}
	return body
}

// extractAutoscaleGroupID pulls the group ID out of a API create response.
func extractAutoscaleGroupID(resp *client.APIResponse) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}
	if id := asIDString(data, "id"); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("autoscale group ID not present in CSA data: %s", string(resp.Data))
}

// decodeAutoscaleGroupList decodes the CSA list response into a slice of maps.
func decodeAutoscaleGroupList(resp *client.APIResponse) ([]map[string]interface{}, error) {
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	var items []map[string]interface{}
	if err := resp.ExtractData(&items); err == nil {
		return items, nil
	}
	// Try wrapper object.
	var wrapper map[string]interface{}
	if err := resp.ExtractData(&wrapper); err != nil {
		return nil, err
	}
	for _, key := range []string{"items", "data", "autoscaleGroups"} {
		if raw, ok := wrapper[key]; ok {
			if arr, ok := raw.([]interface{}); ok {
				result := make([]map[string]interface{}, 0, len(arr))
				for _, v := range arr {
					if m, ok := v.(map[string]interface{}); ok {
						result = append(result, m)
					}
				}
				return result, nil
			}
		}
	}
	return nil, nil
}

// mapAutoscaleGroupResponse populates the resource model from a single CSA list item.
func mapAutoscaleGroupResponse(raw map[string]interface{}, m *AutoscaleGroupResourceModel, vpcID string) {
	if id := asIDString(raw, "id"); id != "" {
		m.ID = types.StringValue(id)
	}
	if v := asString(raw, "name"); v != "" {
		m.Name = types.StringValue(v)
	}
	if v := asIDString(raw, "launchTemplateId"); v != "" {
		m.LaunchTemplateID = types.StringValue(v)
	}
	m.IsAutoscale = types.BoolValue(asBool(raw, "isAutoscale"))
	// Always set int fields — 0 is a valid configuration (e.g. scale-to-zero).
	m.DesiredCapacity = types.Int64Value(asInt64(raw, "desiredCapacity"))
	m.MinSize = types.Int64Value(asInt64(raw, "minSize"))
	m.MaxSize = types.Int64Value(asInt64(raw, "maxSize"))
	if v := asString(raw, "metricType"); v != "" {
		m.MetricType = types.StringValue(v)
	} else if m.MetricType.IsNull() || m.MetricType.IsUnknown() {
		m.MetricType = types.StringValue("CPU")
	}
	m.ScaleOutThreshold = types.Int64Value(asInt64(raw, "scaleOutThreshold"))
	m.ScaleInThreshold = types.Int64Value(asInt64(raw, "scaleInThreshold"))
	m.HasLoadBalancer = types.BoolValue(asBool(raw, "hasLoadBalancer"))
	if v := asString(raw, "vpcId"); v != "" {
		m.VpcID = types.StringValue(v)
	} else {
		m.VpcID = types.StringValue(vpcID)
	}
}
