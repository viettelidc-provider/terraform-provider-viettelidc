// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                   = (*AutoscaleGroupResource)(nil)
	_ resource.ResourceWithConfigure      = (*AutoscaleGroupResource)(nil)
	_ resource.ResourceWithImportState    = (*AutoscaleGroupResource)(nil)
	_ resource.ResourceWithValidateConfig = (*AutoscaleGroupResource)(nil)
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

	// Load-balancer mode (is_autoscale = false). See buildAutoscaleGroupCreateBody.
	LoadBalancerID     types.String `tfsdk:"loadbalancer_id"`
	LoadBalancerPoolID types.String `tfsdk:"loadbalancer_pool_id"`
	SubnetID           types.String `tfsdk:"subnet_id"`
	PortNumber         types.Int64  `tfsdk:"port_number"`
}

// NewAutoscaleGroupResource constructs the resource (registered in iac/provider.go).
func NewAutoscaleGroupResource() resource.Resource { return &AutoscaleGroupResource{} }

func (r *AutoscaleGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_autoscale_group"
}

func (r *AutoscaleGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ViettelIDC Autoscale Group. The API accepts two shapes: with " +
			"is_autoscale the group scales on a metric and needs min_size, max_size " +
			"and both thresholds; with has_load_balancer it is a fixed pool behind an " +
			"existing load balancer and needs loadbalancer_id, loadbalancer_pool_id, " +
			"subnet_id and port_number instead. All attributes are immutable " +
			"(RequiresReplace / ForceNew). Read() uses list+filter because the API has " +
			"no detail endpoint for ASG.",
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
				Optional:    true,
				Description: "Minimum number of instances. Required when is_autoscale is true; omit in load-balancer mode. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"max_size": schema.Int64Attribute{
				Optional:    true,
				Description: "Maximum number of instances. Required when is_autoscale is true; omit in load-balancer mode. Immutable.",
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
				Optional:    true,
				Description: "CPU % threshold to scale out. Required when is_autoscale is true; omit in load-balancer mode. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"scale_in_threshold": schema.Int64Attribute{
				Optional:    true,
				Description: "CPU % threshold to scale in. Required when is_autoscale is true; omit in load-balancer mode. Immutable.",
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
			"loadbalancer_id": schema.StringAttribute{
				Optional:    true,
				Description: "Load balancer to register the group's instances with. Required when has_load_balancer is true. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"loadbalancer_pool_id": schema.StringAttribute{
				Optional:    true,
				Description: "Pool within loadbalancer_id to add members to. Required when has_load_balancer is true. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subnet_id": schema.StringAttribute{
				Optional:    true,
				Description: "Subnet the scaled instances are placed in. Required when has_load_balancer is true. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"port_number": schema.Int64Attribute{
				Optional:    true,
				Description: "Port the pool members listen on. Required when has_load_balancer is true. Immutable.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
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

// ValidateConfig enforces the split between the two shapes the API accepts.
// Without it a config that omits min_size in autoscale mode reaches the API as
// min_size=0 and comes back as an opaque 500.
func (r *AutoscaleGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg AutoscaleGroupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	missing := func(p string, v attr.Value, when string) {
		if v.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(p), "Missing Required Attribute",
				fmt.Sprintf("%s is required when %s.", p, when))
		}
	}

	if cfg.IsAutoscale.ValueBool() {
		for p, v := range map[string]attr.Value{
			"min_size":            cfg.MinSize,
			"max_size":            cfg.MaxSize,
			"scale_out_threshold": cfg.ScaleOutThreshold,
			"scale_in_threshold":  cfg.ScaleInThreshold,
		} {
			missing(p, v, "is_autoscale = true")
		}
	}

	if cfg.HasLoadBalancer.ValueBool() {
		for p, v := range map[string]attr.Value{
			"loadbalancer_id":      cfg.LoadBalancerID,
			"loadbalancer_pool_id": cfg.LoadBalancerPoolID,
			"subnet_id":            cfg.SubnetID,
			"port_number":          cfg.PortNumber,
		} {
			missing(p, v, "has_load_balancer = true")
		}
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
		"name":               plan.Name.ValueString(),
		"launch_template_id": ltIDVal,
		"is_autoscale":       plan.IsAutoscale.ValueBool(),
		"desired_capacity":   plan.DesiredCapacity.ValueInt64(),
		"vpc_id":             vpcID,
		"customer_id":        customerID,
	}

	// The API takes one of two shapes. With is_autoscale the group scales on a
	// metric and carries min/max/thresholds; without it the group is a fixed
	// pool behind a load balancer and the console sends none of those, but does
	// send the four load-balancer fields instead. Sending the wrong set is how
	// the request ends up rejected.
	if plan.IsAutoscale.ValueBool() {
		body["min_size"] = plan.MinSize.ValueInt64()
		body["max_size"] = plan.MaxSize.ValueInt64()
		body["scale_out_threshold"] = plan.ScaleOutThreshold.ValueInt64()
		body["scale_in_threshold"] = plan.ScaleInThreshold.ValueInt64()
		// metric_type defaults to "CPU" when not specified.
		metricType := plan.MetricType.ValueString()
		if metricType == "" {
			metricType = "CPU"
		}
		body["metric_type"] = metricType
	}

	// Only include has_load_balancer when explicitly configured —
	// avoids silently hard-coding false when the user omits the attribute.
	if !plan.HasLoadBalancer.IsNull() && !plan.HasLoadBalancer.IsUnknown() {
		body["has_load_balancer"] = plan.HasLoadBalancer.ValueBool()
	}

	// TODO: add these four to the autoscale-group/create rename list in
	// kong.yaml, then switch them to snake_case like every other field here.
	// Until that is deployed they go out already camelCased so the gateway
	// forwards them untouched.
	if v := plan.LoadBalancerID.ValueString(); v != "" {
		body["loadbalancerId"] = v
	}
	if v := plan.LoadBalancerPoolID.ValueString(); v != "" {
		body["loadbalancerPoolId"] = v
	}
	if v := plan.SubnetID.ValueString(); v != "" {
		// subnetId goes out as an integer; the console sends 9935, not "9935".
		var subnetVal interface{} = v
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			subnetVal = n
		}
		body["subnetId"] = subnetVal
	}
	if !plan.PortNumber.IsNull() && !plan.PortNumber.IsUnknown() {
		body["portNumber"] = plan.PortNumber.ValueInt64()
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
