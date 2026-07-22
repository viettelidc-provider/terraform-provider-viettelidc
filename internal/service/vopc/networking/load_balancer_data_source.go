// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource = (*LoadBalancerDataSource)(nil)
)

type LoadBalancerDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type LoadBalancerDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	SubnetID         types.String `tfsdk:"subnet_id"`
	FloatingIPID     types.String `tfsdk:"floating_ip_id"`
	LoadBalancerType types.String `tfsdk:"loadbalancer_type"`
	PackageType      types.String `tfsdk:"package_type"`
	VpcID            types.String `tfsdk:"vpc_id"`
	AdminStateUp     types.Bool   `tfsdk:"admin_state_up"`
	Status           types.String `tfsdk:"status"`
	OperatingStatus  types.String `tfsdk:"operating_status"`

	IPAddress            types.String `tfsdk:"ip_address"`
	ProvisioningStatus   types.String `tfsdk:"provisioning_status"`
	IsPublicLoadBalancer types.Bool   `tfsdk:"is_public_loadbalancer"`

	Members        []LoadBalancerMemberItem  `tfsdk:"members"`
	HealthMonitors []LoadBalancerMonitorItem `tfsdk:"health_monitors"`

	Listeners types.List `tfsdk:"listeners"`
	Pools     types.List `tfsdk:"pools"`
}

// LoadBalancerMemberItem is one row of member-by-lb/all: what is actually
// behind the load balancer right now, including instances an autoscale group
// added that Terraform never created.
type LoadBalancerMemberItem struct {
	ID                 types.String `tfsdk:"id"`
	PoolID             types.String `tfsdk:"pool_id"`
	PoolName           types.String `tfsdk:"pool_name"`
	Name               types.String `tfsdk:"name"`
	IPAddress          types.String `tfsdk:"ip_address"`
	SubnetID           types.String `tfsdk:"subnet_id"`
	Port               types.Int64  `tfsdk:"port"`
	Weight             types.Int64  `tfsdk:"weight"`
	Backup             types.Bool   `tfsdk:"backup"`
	Status             types.String `tfsdk:"status"`
	OperatingStatus    types.String `tfsdk:"operating_status"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

// LoadBalancerMonitorItem is one row of monitor-by-lb/all.
type LoadBalancerMonitorItem struct {
	ID                 types.String `tfsdk:"id"`
	PoolID             types.String `tfsdk:"pool_id"`
	PoolName           types.String `tfsdk:"pool_name"`
	Name               types.String `tfsdk:"name"`
	Type               types.String `tfsdk:"type"`
	Delay              types.Int64  `tfsdk:"delay"`
	Timeout            types.Int64  `tfsdk:"timeout"`
	MaxRetries         types.Int64  `tfsdk:"max_retries"`
	MaxRetriesDown     types.Int64  `tfsdk:"max_retries_down"`
	Status             types.String `tfsdk:"status"`
	OperatingStatus    types.String `tfsdk:"operating_status"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

func NewLoadBalancerDataSource() datasource.DataSource { return &LoadBalancerDataSource{} }

func (d *LoadBalancerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_load_balancer"
}

func (d *LoadBalancerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
		Description: "Lookup a Load Balancer by ID, name or IP address in a VPC.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Load Balancer ID (vttLoadBalancerId). One of 'id', 'name' or 'ip_address' must be specified.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the Load Balancer to look up. One of 'id', 'name' or 'ip_address' must be specified.",
			},
			"vpc_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "VPC ID to search within. Uses provider default if not specified.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Description of the Load Balancer.",
			},
			"subnet_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the subnet where the Load Balancer is placed.",
			},
			"floating_ip_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the floating IP assigned to the Load Balancer.",
			},
			"loadbalancer_type": schema.StringAttribute{
				Computed:    true,
				Description: "Type of the Load Balancer.",
			},
			"package_type": schema.StringAttribute{
				Computed:    true,
				Description: "Package type of the Load Balancer.",
			},
			"admin_state_up": schema.BoolAttribute{
				Computed:    true,
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
			"ip_address": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Private IP the Load Balancer listens on. Can also be used to look one up.",
			},
			"provisioning_status": schema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status of the Load Balancer.",
			},
			"is_public_loadbalancer": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the Load Balancer is reachable from the internet.",
			},
			"members": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Pool members currently behind the Load Balancer. Unlike the resource's pool_members, which records what was asked for at creation, this is what the API reports right now.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                  schema.StringAttribute{Computed: true, Description: "Member ID."},
						"pool_id":             schema.StringAttribute{Computed: true, Description: "Pool the member belongs to."},
						"pool_name":           schema.StringAttribute{Computed: true, Description: "Pool name."},
						"name":                schema.StringAttribute{Computed: true, Description: "Member name, usually the VM name."},
						"ip_address":          schema.StringAttribute{Computed: true, Description: "Member IP."},
						"subnet_id":           schema.StringAttribute{Computed: true, Description: "Subnet the member sits in."},
						"port":                schema.Int64Attribute{Computed: true, Description: "Port traffic is forwarded to."},
						"weight":              schema.Int64Attribute{Computed: true, Description: "Load balancing weight."},
						"backup":              schema.BoolAttribute{Computed: true, Description: "Whether this is a backup member."},
						"status":              schema.StringAttribute{Computed: true, Description: "Member status."},
						"operating_status":    schema.StringAttribute{Computed: true, Description: "Operating status."},
						"provisioning_status": schema.StringAttribute{Computed: true, Description: "Provisioning status."},
					},
				},
			},
			"health_monitors": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Health monitors attached to the Load Balancer's pools.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                  schema.StringAttribute{Computed: true, Description: "Health monitor ID."},
						"pool_id":             schema.StringAttribute{Computed: true, Description: "Pool the monitor checks."},
						"pool_name":           schema.StringAttribute{Computed: true, Description: "Pool name."},
						"name":                schema.StringAttribute{Computed: true, Description: "Monitor name."},
						"type":                schema.StringAttribute{Computed: true, Description: "Check type, for example PING."},
						"delay":               schema.Int64Attribute{Computed: true, Description: "Seconds between checks."},
						"timeout":             schema.Int64Attribute{Computed: true, Description: "Seconds before a check times out."},
						"max_retries":         schema.Int64Attribute{Computed: true, Description: "Successes needed to mark a member up."},
						"max_retries_down":    schema.Int64Attribute{Computed: true, Description: "Failures needed to mark a member down."},
						"status":              schema.StringAttribute{Computed: true, Description: "Monitor status."},
						"operating_status":    schema.StringAttribute{Computed: true, Description: "Operating status."},
						"provisioning_status": schema.StringAttribute{Computed: true, Description: "Provisioning status."},
					},
				},
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
		},
	}
}

func (d *LoadBalancerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LoadBalancerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config LoadBalancerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, diags := d.lookup(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch listeners and pools using helper methods
	d.fetchListeners(ctx, result, &resp.Diagnostics)
	d.fetchPools(ctx, result, &resp.Diagnostics)
	d.fetchMembers(ctx, result, &resp.Diagnostics)
	d.fetchHealthMonitors(ctx, result, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, result)...)
}

// lookup finds one load balancer by whichever selector the config set. Split
// out of Read so the matching can be tested without a ReadRequest.
func (d *LoadBalancerDataSource) lookup(ctx context.Context, config *LoadBalancerDataSourceModel) (*LoadBalancerDataSourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	vpcID := defaultIfEmpty(config.VpcID, d.defaultVpcID)
	if vpcID == "" {
		diags.AddError("Missing vpc_id", "Set 'vpc_id' or configure provider default.")
		return nil, diags
	}

	if config.ID.IsNull() && config.Name.IsNull() && config.IPAddress.IsNull() {
		diags.AddError("Missing filter", "One of 'id', 'name' or 'ip_address' must be specified.")
		return nil, diags
	}

	// ponytail: the endpoint also filters server-side, e.g.
	// filters:[{"name":"LBTbl.name","values":["dev"]}] or {"name":"ip_address"}.
	// One page of 1000 covers every VPC we have seen; switch to the server
	// filter if one ever holds more.
	body := map[string]interface{}{
		"vpc_id":      vpcID,
		"customer_id": d.customerID,
		"pageIndex":   0,
		"pageSize":    1000,
		"filters":     []interface{}{},
	}

	apiResp, d2 := callAPI(ctx, d.client, pathLoadBalancerList, body)
	diags.Append(d2...)
	if diags.HasError() {
		return nil, diags
	}

	type lbListItem struct {
		VttLoadBalancerID       int64  `json:"vttLoadBalancerId"`
		Name                    string `json:"name"`
		Description             string `json:"description"`
		VttSubnetID             int64  `json:"vttSubnetId"`
		VttLoadbalancerTypeName string `json:"vttLoadbalancerTypeName"`
		LoadbalancerTypeName    string `json:"loadbalancerTypeName"`
		AdminStateUp            bool   `json:"adminStateUp"`
		Status                  string `json:"status"`
		OperatingStatus         string `json:"operatingStatus"`
		IPAddress               string `json:"ipAddress"`
		ProvisioningStatus      string `json:"provisioningStatus"`
		IsPublicLoadbalancer    bool   `json:"isPublicLoadbalancer"`
	}
	var listResp struct {
		Items []lbListItem `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return nil, diags
	}

	var found *lbListItem
	for i := range listResp.Items {
		item := &listResp.Items[i]
		if !config.ID.IsNull() && fmt.Sprintf("%d", item.VttLoadBalancerID) == config.ID.ValueString() {
			found = item
			break
		}
		if !config.Name.IsNull() && item.Name == config.Name.ValueString() {
			found = item
			break
		}
		if !config.IPAddress.IsNull() && item.IPAddress == config.IPAddress.ValueString() {
			found = item
			break
		}
	}

	if found == nil {
		diags.AddError("Not Found", fmt.Sprintf("Load Balancer not found with id=%s name=%s ip_address=%s",
			config.ID.ValueString(), config.Name.ValueString(), config.IPAddress.ValueString()))
		return nil, diags
	}

	return &LoadBalancerDataSourceModel{
		ID:                   types.StringValue(fmt.Sprintf("%d", found.VttLoadBalancerID)),
		Name:                 types.StringValue(found.Name),
		Description:          types.StringValue(found.Description),
		VpcID:                types.StringValue(vpcID),
		SubnetID:             types.StringValue(fmt.Sprintf("%d", found.VttSubnetID)),
		FloatingIPID:         types.StringValue(""),
		LoadBalancerType:     types.StringValue(found.VttLoadbalancerTypeName),
		PackageType:          types.StringValue(found.LoadbalancerTypeName),
		AdminStateUp:         types.BoolValue(found.AdminStateUp),
		Status:               types.StringValue(found.Status),
		IPAddress:            types.StringValue(found.IPAddress),
		ProvisioningStatus:   types.StringValue(found.ProvisioningStatus),
		IsPublicLoadBalancer: types.BoolValue(found.IsPublicLoadbalancer),
		OperatingStatus:      types.StringValue(found.OperatingStatus),
	}, diags
}

func (d *LoadBalancerDataSource) fetchListeners(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerListeners, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		Protocol        string `json:"protocol"`
		ProtocolPort    int    `json:"protocolPort"`
		XForwardedFor   bool   `json:"xForwardedFor"`
		XForwardedPort  bool   `json:"xForwardedPort"`
		XForwardedProto bool   `json:"xForwardedProto"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var listeners []attr.Value
	for _, item := range listResp {
		listenerMap := map[string]attr.Value{
			"id":                types.StringValue(item.ID),
			"name":              types.StringValue(item.Name),
			"description":       types.StringValue(item.Description),
			"protocol":          types.StringValue(item.Protocol),
			"protocol_port":     types.Int64Value(int64(item.ProtocolPort)),
			"x_forwarded_for":   types.BoolValue(item.XForwardedFor),
			"x_forwarded_port":  types.BoolValue(item.XForwardedPort),
			"x_forwarded_proto": types.BoolValue(item.XForwardedProto),
		}
		obj, objDiags := types.ObjectValue(map[string]attr.Type{
			"id":                types.StringType,
			"name":              types.StringType,
			"description":       types.StringType,
			"protocol":          types.StringType,
			"protocol_port":     types.Int64Type,
			"x_forwarded_for":   types.BoolType,
			"x_forwarded_port":  types.BoolType,
			"x_forwarded_proto": types.BoolType,
		}, listenerMap)
		diags.Append(objDiags...)
		listeners = append(listeners, obj)
	}

	listType, listDiags := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}}, listeners)
	diags.Append(listDiags...)
	model.Listeners = listType
}

func (d *LoadBalancerDataSource) fetchPools(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}

	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerPools, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID                     string `json:"id"`
		Name                   string `json:"name"`
		Description            string `json:"description"`
		Algorithm              string `json:"algorithm"`
		SessionPersistenceType string `json:"sessionPersistenceType"`
	}

	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	var pools []attr.Value
	for _, item := range listResp {
		poolMap := map[string]attr.Value{
			"id":                       types.StringValue(item.ID),
			"name":                     types.StringValue(item.Name),
			"description":              types.StringValue(item.Description),
			"algorithm":                types.StringValue(item.Algorithm),
			"session_persistence_type": types.StringValue(item.SessionPersistenceType),
		}
		obj, objDiags := types.ObjectValue(map[string]attr.Type{
			"id":                       types.StringType,
			"name":                     types.StringType,
			"description":              types.StringType,
			"algorithm":                types.StringType,
			"session_persistence_type": types.StringType,
		}, poolMap)
		diags.Append(objDiags...)
		pools = append(pools, obj)
	}

	listType, listDiags := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}}, pools)
	diags.Append(listDiags...)
	model.Pools = listType
}

// fetchMembers reads member-by-lb/all. The route was missing from kong.yaml
// until this change, so an older gateway answers 404 here.
func (d *LoadBalancerDataSource) fetchMembers(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}
	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerMembers, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID                    string `json:"id"`
		VttLoadBalancerPoolID int64  `json:"vttLoadBalancerPoolId"`
		PoolName              string `json:"poolName"`
		Name                  string `json:"name"`
		IPAddress             string `json:"ipAddress"`
		VttSubnetID           int64  `json:"vttSubnetId"`
		Port                  int64  `json:"port"`
		Weight                int64  `json:"weight"`
		Backup                bool   `json:"backup"`
		Status                string `json:"status"`
		OperatingStatus       string `json:"operatingStatus"`
		ProvisioningStatus    string `json:"provisioningStatus"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	model.Members = make([]LoadBalancerMemberItem, 0, len(listResp))
	for _, it := range listResp {
		model.Members = append(model.Members, LoadBalancerMemberItem{
			ID:                 types.StringValue(it.ID),
			PoolID:             types.StringValue(fmt.Sprintf("%d", it.VttLoadBalancerPoolID)),
			PoolName:           types.StringValue(it.PoolName),
			Name:               types.StringValue(it.Name),
			IPAddress:          types.StringValue(it.IPAddress),
			SubnetID:           types.StringValue(fmt.Sprintf("%d", it.VttSubnetID)),
			Port:               types.Int64Value(it.Port),
			Weight:             types.Int64Value(it.Weight),
			Backup:             types.BoolValue(it.Backup),
			Status:             types.StringValue(it.Status),
			OperatingStatus:    types.StringValue(it.OperatingStatus),
			ProvisioningStatus: types.StringValue(it.ProvisioningStatus),
		})
	}
}

// fetchHealthMonitors reads monitor-by-lb/all, same caveat about the route.
func (d *LoadBalancerDataSource) fetchHealthMonitors(ctx context.Context, model *LoadBalancerDataSourceModel, diags *diag.Diagnostics) {
	body := map[string]interface{}{
		"vpc_id":            model.VpcID.ValueString(),
		"customer_id":       d.customerID,
		"vttLoadBalancerId": parseInt(model.ID.ValueString()),
	}
	apiResp, callDiags := callAPI(ctx, d.client, pathLoadBalancerMonitors, body)
	diags.Append(callDiags...)
	if diags.HasError() {
		return
	}

	var listResp []struct {
		ID                    string `json:"id"`
		VttLoadBalancerPoolID int64  `json:"vttLoadBalancerPoolId"`
		PoolName              string `json:"poolName"`
		Name                  string `json:"name"`
		Type                  string `json:"type"`
		Delay                 int64  `json:"delay"`
		Timeout               int64  `json:"timeout"`
		MaxRetries            int64  `json:"maxRetries"`
		MaxRetriesDown        int64  `json:"maxRetriesDown"`
		Status                string `json:"status"`
		OperatingStatus       string `json:"operatingStatus"`
		ProvisioningStatus    string `json:"provisioningStatus"`
	}
	if err := json.Unmarshal(apiResp.Data, &listResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	model.HealthMonitors = make([]LoadBalancerMonitorItem, 0, len(listResp))
	for _, it := range listResp {
		model.HealthMonitors = append(model.HealthMonitors, LoadBalancerMonitorItem{
			ID:                 types.StringValue(it.ID),
			PoolID:             types.StringValue(fmt.Sprintf("%d", it.VttLoadBalancerPoolID)),
			PoolName:           types.StringValue(it.PoolName),
			Name:               types.StringValue(it.Name),
			Type:               types.StringValue(it.Type),
			Delay:              types.Int64Value(it.Delay),
			Timeout:            types.Int64Value(it.Timeout),
			MaxRetries:         types.Int64Value(it.MaxRetries),
			MaxRetriesDown:     types.Int64Value(it.MaxRetriesDown),
			Status:             types.StringValue(it.Status),
			OperatingStatus:    types.StringValue(it.OperatingStatus),
			ProvisioningStatus: types.StringValue(it.ProvisioningStatus),
		})
	}
}
