// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                = (*LoadBalancerResource)(nil)
	_ resource.ResourceWithConfigure   = (*LoadBalancerResource)(nil)
	_ resource.ResourceWithImportState = (*LoadBalancerResource)(nil)
)

type LoadBalancerResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type LoadBalancerResourceModel struct {
	ID               types.String      `tfsdk:"id"`
	Name             types.String      `tfsdk:"name"`
	Description      types.String      `tfsdk:"description"`
	SubnetID         types.String      `tfsdk:"subnet_id"`
	FloatingIPID     types.String      `tfsdk:"floating_ip_id"`
	LoadBalancerType types.String      `tfsdk:"loadbalancer_type"`
	PackageType      types.String      `tfsdk:"package_type"`
	VpcID            types.String      `tfsdk:"vpc_id"`
	AdminStateUp     types.Bool        `tfsdk:"admin_state_up"`
	Status           types.String      `tfsdk:"status"`
	OperatingStatus  types.String      `tfsdk:"operating_status"`
	Listeners        types.List        `tfsdk:"listeners"`
	Pools            types.List        `tfsdk:"pools"`
	PoolMembers      []PoolMemberInput `tfsdk:"pool_members"`
}

type PoolMemberInput struct {
	VmID   types.String `tfsdk:"vm_id"`
	Port   types.Int64  `tfsdk:"port"`
	Weight types.Int64  `tfsdk:"weight"`
}

type ListenerModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Protocol        types.String `tfsdk:"protocol"`
	ProtocolPort    types.Int64  `tfsdk:"protocol_port"`
	XForwardedFor   types.Bool   `tfsdk:"x_forwarded_for"`
	XForwardedPort  types.Bool   `tfsdk:"x_forwarded_port"`
	XForwardedProto types.Bool   `tfsdk:"x_forwarded_proto"`
}

type PoolModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	Algorithm              types.String `tfsdk:"algorithm"`
	SessionPersistenceType types.String `tfsdk:"session_persistence_type"`
}

func NewLoadBalancerResource() resource.Resource { return &LoadBalancerResource{} }

func (r *LoadBalancerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_load_balancer"
}

func (r *LoadBalancerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	listenerAttrTypes := map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}

	poolAttrTypes := map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}

	resp.Schema = schema.Schema{
		Description: "ViettelIDC Load Balancer for distributing traffic across multiple instances.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Load Balancer ID assigned by the system (vttLoadBalancerId).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable Load Balancer name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Description of the Load Balancer.",
			},
			"subnet_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the subnet where the Load Balancer will be placed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"floating_ip_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the floating IP to assign to the Load Balancer.",
			},
			"loadbalancer_type": schema.StringAttribute{
				Required:    true,
				Description: "Type of Load Balancer (e.g., 'APPLICATION HTTP-HTTPS').",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"package_type": schema.StringAttribute{
				Required:    true,
				Description: "Package type of the Load Balancer (e.g., 'LB Compact').",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID. Uses provider default if not specified.",
			},
			"admin_state_up": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Administrative state of the Load Balancer.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status of the Load Balancer.",
			},
			"operating_status": schema.StringAttribute{
				Computed:    true,
				Description: "Operating status of the Load Balancer.",
			},
			"listeners": schema.ListAttribute{
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: listenerAttrTypes},
				Description: "List of listeners associated with the Load Balancer.",
			},
			"pools": schema.ListAttribute{
				Computed:    true,
				ElementType: types.ObjectType{AttrTypes: poolAttrTypes},
				Description: "List of pools associated with the Load Balancer.",
			},
			"pool_members": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Backend VMs to add as pool members at creation time. Changing this requires replacement.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vm_id": schema.StringAttribute{
							Required:    true,
							Description: "VM ID to add as a pool member.",
						},
						"port": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(80),
							Description: "Port on the VM to forward traffic to (default 80).",
						},
						"weight": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Default:     int64default.StaticInt64(1),
							Description: "Weight for this member in load balancing (default 1).",
						},
					},
				},
			},
		},
	}
}

func (r *LoadBalancerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = pd.Client
	r.customerID = pd.CustomerID
	r.defaultVpcID = pd.DefaultVpcID
}

func (r *LoadBalancerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LoadBalancerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := defaultIfEmpty(plan.VpcID, r.defaultVpcID)
	if vpcID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("vpc_id"), "Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return
	}

	// Map package type to numeric code used by the API
	lbTypeCode, err := getPackageTypeCode(plan.PackageType.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("package_type"), "Invalid package_type", err.Error())
		return
	}

	// Resolve pool members (calls attached-nic/list) before building request body.
	members := r.buildMembers(ctx, plan.PoolMembers, vpcID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Use snake_case vpc_id/customer_id so the API Gateway renames them
	// to vpcId/customerId before forwarding to API. callAPI also converts the string
	// values to integers. Sending camelCase directly causes SERVICE_ENDPOINT_BODY_INCORRECT.
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
		"loadBalancer": map[string]interface{}{
			"name":                    plan.Name.ValueString(),
			"description":             plan.Description.ValueString(),
			"vttLoadbalancerTypeName": plan.LoadBalancerType.ValueString(),
			"loadbalancerType":        lbTypeCode,
			"vttSubnetId":             parseInt(plan.SubnetID.ValueString()),
			"vttFloatingId":           parseIntPtr(plan.FloatingIPID.ValueString()),
			"vpcId":                   parseInt(vpcID),
			"lbType":                  plan.LoadBalancerType.ValueString(),
			"packageType":             plan.PackageType.ValueString(),
		},
		"listener": map[string]interface{}{
			"name":            plan.Name.ValueString() + "-listener",
			"protocol":        "HTTP",
			"protocolPort":    80,
			"xForwardedFor":   false,
			"xForwardedPort":  false,
			"xForwardedProto": false,
		},
		"pool": map[string]interface{}{
			"name":                   plan.Name.ValueString() + "-pool",
			"algorithm":              "ROUND_ROBIN",
			"sessionPersistenceType": "NONE",
			"vpcId":                  parseInt(vpcID),
		},
		"members": members,
		"monitor": map[string]interface{}{
			"name":           plan.Name.ValueString() + "-health",
			"type":           "HTTP",
			"maxRetriesDown": 3,
			"delay":          5,
			"maxRetries":     3,
			"timeout":        5,
			"httpMethod":     "GET",
			"expectedCode":   200,
			"urlPath":        "/",
		},
	}

	plan.VpcID = types.StringValue(vpcID)

	listBody := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
	}

	// Pre-check: adopt existing LB with same name (handles re-run after
	// a failed apply that created the LB but did not persist state).
	var actualLBID int64
	var skipPoll bool
	if preResp, _ := callAPI(ctx, r.client, pathLoadBalancerList, listBody); preResp != nil {
		var lr struct {
			Items []struct {
				VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
				Name              string `json:"name"`
				Status            string `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal(preResp.Data, &lr); err == nil {
			for _, item := range lr.Items {
				if item.Name == plan.Name.ValueString() {
					actualLBID = item.VttLoadBalancerID
					s := strings.ToUpper(item.Status)
					skipPoll = (s == "SUCCESS" || s == "ACTIVE")
					break
				}
			}
		}
	}

	if actualLBID == 0 {
		// LB does not exist yet — create it via compound-create.
		_, diags := callAPI(ctx, r.client, pathLoadBalancerCreate, body)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		// Poll the list endpoint to find the newly created LB by name (up to ~150s).
		for attempt := 0; attempt < 30; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			listCSAResp, listDiags := callAPI(ctx, r.client, pathLoadBalancerList, listBody)
			if listDiags.HasError() {
				resp.Diagnostics.Append(listDiags...)
				return
			}
			var listResult struct {
				Items []struct {
					VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
					Name              string `json:"name"`
				} `json:"items"`
			}
			if err := json.Unmarshal(listCSAResp.Data, &listResult); err != nil {
				resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("parse LB list after create: %s", err))
				return
			}
			for _, item := range listResult.Items {
				if item.Name == plan.Name.ValueString() {
					actualLBID = item.VttLoadBalancerID
					break
				}
			}
			if actualLBID != 0 {
				break
			}
		}
		if actualLBID == 0 {
			resp.Diagnostics.AddError("Create Error", "LB '"+plan.Name.ValueString()+"' not found in VPC "+vpcID+" after creation")
			return
		}
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", actualLBID))

	// Initialize computed fields to empty (not unknown) before saving partial
	// state — Terraform requires all values to be known after apply.
	if plan.Status.IsUnknown() {
		plan.Status = types.StringValue("")
	}
	if plan.OperatingStatus.IsUnknown() {
		plan.OperatingStatus = types.StringValue("")
	}
	if plan.Listeners.IsUnknown() {
		plan.Listeners, _ = types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
			"id": types.StringType, "name": types.StringType, "description": types.StringType,
			"protocol": types.StringType, "protocol_port": types.Int64Type,
			"x_forwarded_for": types.BoolType, "x_forwarded_port": types.BoolType, "x_forwarded_proto": types.BoolType,
		}}, []attr.Value{})
	}
	if plan.Pools.IsUnknown() {
		plan.Pools, _ = types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
			"id": types.StringType, "name": types.StringType, "description": types.StringType,
			"algorithm": types.StringType, "session_persistence_type": types.StringType,
		}}, []attr.Value{})
	}

	// Save ID to state immediately so that if polling (or any subsequent step)
	// fails, the resource is tracked and can be cleaned up with terraform destroy.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !skipPoll {
		// Poll the LIST endpoint until this LB's status is success/active.
		// Using list (not detail) for reliability during early provisioning.
		pollDeadline := time.Now().Add(25 * time.Minute)
		pollDone := false
		for !pollDone {
			listCSAResp, _ := callAPI(ctx, r.client, pathLoadBalancerList, listBody)
			if listCSAResp != nil {
				tflog.Debug(ctx, "LB poll response", map[string]interface{}{"raw": string(listCSAResp.Data)})
				var listResult struct {
					Items []struct {
						VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
						Status            string `json:"status"`
					} `json:"items"`
				}
				if err := json.Unmarshal(listCSAResp.Data, &listResult); err == nil {
					for _, item := range listResult.Items {
						if item.VttLoadBalancerID == actualLBID {
							s := strings.ToUpper(item.Status)
							if s == "SUCCESS" || s == "ACTIVE" {
								pollDone = true
							} else if s == "ERROR" || s == "FAILED" {
								resp.Diagnostics.AddError("LB Error", fmt.Sprintf("Load Balancer %d entered error state: %s", actualLBID, item.Status))
								return
							}
							break
						}
					}
				}
			}
			if pollDone {
				break
			}
			if time.Now().After(pollDeadline) {
				resp.Diagnostics.AddError("Load Balancer did not become ready", fmt.Sprintf("timed out waiting for LB %d (timeout=25m)", actualLBID))
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Context cancelled", ctx.Err().Error())
				return
			case <-time.After(10 * time.Second):
			}
		}
	}

	// Fetch details to get computed fields
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *LoadBalancerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LoadBalancerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readAndMerge(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *LoadBalancerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LoadBalancerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only adminStateUp can be updated via API
	body := map[string]interface{}{
		"vpc_id":            plan.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(plan.ID.ValueString()),
		"adminStateUp":      plan.AdminStateUp.ValueBool(),
	}

	_, diags := callAPI(ctx, r.client, pathLoadBalancerUpdate, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-fetch details
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *LoadBalancerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LoadBalancerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"vpc_id":            state.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(state.ID.ValueString()),
	}

	apiResp, diags := callAPI(ctx, r.client, pathLoadBalancerDelete, body)
	if diags.HasError() {
		if apiResp != nil && isNotFoundMessage(apiResp.Message) {
			return
		}
		resp.Diagnostics.Append(diags...)
		return
	}

	// Poll until LB is fully gone from the API (delete is async).
	pollBody := map[string]interface{}{
		"vpc_id":            state.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(state.ID.ValueString()),
	}
	if err := pollUntilGone(ctx, r.client, pathLoadBalancerDetail, pollBody, 10*time.Minute); err != nil {
		resp.Diagnostics.AddError("Load Balancer did not disappear after delete", err.Error())
	}
}

func (r *LoadBalancerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *LoadBalancerResource) readAndMerge(ctx context.Context, model *LoadBalancerResourceModel, diags *diag.Diagnostics) {
	if model.VpcID.ValueString() == "" || model.ID.ValueString() == "" {
		return
	}

	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathLoadBalancerDetail, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var detailResp struct {
		VttLoadBalancerID int64  `json:"vttLoadBalancerId"`
		Name              string `json:"name"`
		Description       string `json:"description"`
		VttSubnetID       int64  `json:"vttSubnetId"`
		LoadBalancerType  string `json:"vttLoadbalancerTypeName"`
		PackageType       string `json:"loadbalancerTypeName"`
		AdminStateUp      bool   `json:"adminStateUp"`
		Status            string `json:"status"`
		OperatingStatus   string `json:"operatingStatus"`
	}

	if err := json.Unmarshal(apiResp.Data, &detailResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	// If the load balancer is in a terminal error state, record the status
	// and return without populating other fields. This allows terraform destroy
	// to proceed — Delete() will call the delete API regardless of status.
	if st := strings.ToUpper(detailResp.Status); st == "ERROR" || st == "FAILED" {
		model.Status = types.StringValue(detailResp.Status)
		return
	}

	model.Name = types.StringValue(detailResp.Name)
	model.Description = types.StringValue(detailResp.Description)
	model.SubnetID = types.StringValue(fmt.Sprintf("%d", detailResp.VttSubnetID))
	model.LoadBalancerType = types.StringValue(detailResp.LoadBalancerType)
	// Preserve the user-configured PackageType if the API maps to the same lb type code.
	// API normalizes "LB Small" → "LB Compact" (both map to lbTypeCode=1).
	apiPkg := detailResp.PackageType
	existingPkg := model.PackageType.ValueString()
	if apiPkg != existingPkg && !model.PackageType.IsNull() && !model.PackageType.IsUnknown() {
		// If both values resolve to the same lbTypeCode, keep the plan/state value.
		codeFn := func(s string) int {
			code, _ := getPackageTypeCode(s)
			return code
		}
		if codeFn(existingPkg) == codeFn(apiPkg) {
			apiPkg = existingPkg
		}
	}
	model.PackageType = types.StringValue(apiPkg)
	model.AdminStateUp = types.BoolValue(detailResp.AdminStateUp)
	model.Status = types.StringValue(detailResp.Status)
	model.OperatingStatus = types.StringValue(detailResp.OperatingStatus)

	// Fetch listeners
	r.fetchListeners(ctx, model, diags)
	// Fetch pools
	r.fetchPools(ctx, model, diags)
}

func (r *LoadBalancerResource) fetchListeners(ctx context.Context, model *LoadBalancerResourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathLoadBalancerListeners, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID              interface{} `json:"id"`
		Name            string      `json:"name"`
		Description     string      `json:"description"`
		Protocol        string      `json:"protocol"`
		ProtocolPort    int         `json:"protocolPort"`
		XForwardedFor   bool        `json:"xForwardedFor"`
		XForwardedPort  bool        `json:"xForwardedPort"`
		XForwardedProto bool        `json:"xForwardedProto"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var listeners []attr.Value
	for _, item := range listResp {
		var idStr string
		switch v := item.ID.(type) {
		case string:
			idStr = v
		case float64:
			idStr = fmt.Sprintf("%d", int64(v))
		case int64:
			idStr = fmt.Sprintf("%d", v)
		case int:
			idStr = fmt.Sprintf("%d", v)
		default:
			idStr = fmt.Sprintf("%v", v)
		}

		listenerMap := map[string]attr.Value{
			"id":                types.StringValue(idStr),
			"name":              types.StringValue(item.Name),
			"description":       types.StringValue(item.Description),
			"protocol":          types.StringValue(item.Protocol),
			"protocol_port":     types.Int64Value(int64(item.ProtocolPort)),
			"x_forwarded_for":   types.BoolValue(item.XForwardedFor),
			"x_forwarded_port":  types.BoolValue(item.XForwardedPort),
			"x_forwarded_proto": types.BoolValue(item.XForwardedProto),
		}
		obj, d := types.ObjectValue(map[string]attr.Type{
			"id":                types.StringType,
			"name":              types.StringType,
			"description":       types.StringType,
			"protocol":          types.StringType,
			"protocol_port":     types.Int64Type,
			"x_forwarded_for":   types.BoolType,
			"x_forwarded_port":  types.BoolType,
			"x_forwarded_proto": types.BoolType,
		}, listenerMap)
		diags.Append(d...)
		listeners = append(listeners, obj)
	}

	listType, d := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}}, listeners)
	diags.Append(d...)
	model.Listeners = listType
}

func (r *LoadBalancerResource) fetchPools(ctx context.Context, model *LoadBalancerResourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       r.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, r.client, pathLoadBalancerPools, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID                     interface{} `json:"id"`
		Name                   string      `json:"name"`
		Description            string      `json:"description"`
		Algorithm              string      `json:"algorithm"`
		SessionPersistenceType string      `json:"sessionPersistenceType"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var pools []attr.Value
	for _, item := range listResp {
		var idStr string
		switch v := item.ID.(type) {
		case string:
			idStr = v
		case float64:
			idStr = fmt.Sprintf("%d", int64(v))
		case int64:
			idStr = fmt.Sprintf("%d", v)
		case int:
			idStr = fmt.Sprintf("%d", v)
		default:
			idStr = fmt.Sprintf("%v", v)
		}

		poolMap := map[string]attr.Value{
			"id":                       types.StringValue(idStr),
			"name":                     types.StringValue(item.Name),
			"description":              types.StringValue(item.Description),
			"algorithm":                types.StringValue(item.Algorithm),
			"session_persistence_type": types.StringValue(item.SessionPersistenceType),
		}
		obj, d := types.ObjectValue(map[string]attr.Type{
			"id":                       types.StringType,
			"name":                     types.StringType,
			"description":              types.StringType,
			"algorithm":                types.StringType,
			"session_persistence_type": types.StringType,
		}, poolMap)
		diags.Append(d...)
		pools = append(pools, obj)
	}

	listType, d := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}}, pools)
	diags.Append(d...)
	model.Pools = listType
}

// buildMembers resolves NIC details for each pool_members entry by calling
// attached-nic/list, then returns the members array for compound-create.
// Returns an empty slice (never nil) so the body always has a "members" key.
func (r *LoadBalancerResource) buildMembers(ctx context.Context, inputs []PoolMemberInput, vpcID string, diags *diag.Diagnostics) []map[string]interface{} {
	if len(inputs) == 0 {
		return []map[string]interface{}{}
	}

	nicBody := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	nicResp, callDiags := callAPI(ctx, r.client, pathLoadBalancerAttachedNic, nicBody)
	diags.Append(callDiags...)
	if diags.HasError() {
		return []map[string]interface{}{}
	}

	var nics []map[string]interface{}
	if err := json.Unmarshal(nicResp.Data, &nics); err != nil {
		diags.AddError("Parse Error", "failed to parse attached NICs: "+err.Error())
		return []map[string]interface{}{}
	}

	var members []map[string]interface{}
	for _, pm := range inputs {
		vmID := parseInt(pm.VmID.ValueString())
		var matched map[string]interface{}
		// Prefer root NIC
		for _, nic := range nics {
			ev, _ := nic["vttEntityValue"].(float64)
			ir, _ := nic["isRootNic"].(bool)
			if int64(ev) == vmID && ir {
				matched = nic
				break
			}
		}
		if matched == nil {
			// Fallback: any NIC for this VM
			for _, nic := range nics {
				ev, _ := nic["vttEntityValue"].(float64)
				if int64(ev) == vmID {
					matched = nic
					break
				}
			}
		}
		if matched == nil {
			diags.AddError("Member Not Found",
				fmt.Sprintf("no attached NIC found for vm_id=%s in VPC %s — ensure the VM exists and is in the same VPC", pm.VmID.ValueString(), vpcID))
			return []map[string]interface{}{}
		}
		// Override port/weight from user input
		matched["port"] = pm.Port.ValueInt64()
		matched["weight"] = pm.Weight.ValueInt64()
		members = append(members, matched)
	}
	return members
}

func parseInt(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func parseIntPtr(s string) *int64 {
	if s == "" {
		return nil
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

func getPackageTypeCode(s string) (int, error) {
	switch s {
	case "LB Compact", "LB Small":
		return 1, nil
	case "LB Large":
		return 2, nil
	case "LB Quad Large":
		return 3, nil
	case "LB X-Large", "LB X Large":
		return 4, nil
	case "LB Large HA":
		return 5, nil
	case "LB Compact HA":
		return 6, nil
	case "LB X Large HA", "LB X-Large HA":
		return 7, nil
	case "LB Quad Large HA":
		return 8, nil
	case "LB K8S Base":
		return 9, nil
	default:
		return 0, fmt.Errorf("invalid load balancer package type: %q. Supported types are: 'LB Compact', 'LB Small', 'LB Large', 'LB Quad Large', 'LB X-Large', 'LB Large HA', 'LB Compact HA', 'LB X Large HA', 'LB Quad Large HA', 'LB K8S Base'", s)
	}
}
