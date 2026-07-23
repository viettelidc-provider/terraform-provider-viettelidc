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

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ resource.Resource                   = (*LoadBalancerResource)(nil)
	_ resource.ResourceWithConfigure      = (*LoadBalancerResource)(nil)
	_ resource.ResourceWithImportState    = (*LoadBalancerResource)(nil)
	_ resource.ResourceWithValidateConfig = (*LoadBalancerResource)(nil)
)

type LoadBalancerResource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

type LoadBalancerResourceModel struct {
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

	ListenerName           types.String `tfsdk:"listener_name"`
	ListenerProtocol       types.String `tfsdk:"listener_protocol"`
	ListenerPort           types.Int64  `tfsdk:"listener_port"`
	CertificateID          types.String `tfsdk:"certificate_id"`
	PoolName               types.String `tfsdk:"pool_name"`
	PoolAlgorithm          types.String `tfsdk:"pool_algorithm"`
	PoolSessionPersistence types.String `tfsdk:"pool_session_persistence"`

	MonitorName           types.String      `tfsdk:"monitor_name"`
	MonitorType           types.String      `tfsdk:"monitor_type"`
	MonitorDelay          types.Int64       `tfsdk:"monitor_delay"`
	MonitorTimeout        types.Int64       `tfsdk:"monitor_timeout"`
	MonitorMaxRetries     types.Int64       `tfsdk:"monitor_max_retries"`
	MonitorMaxRetriesDown types.Int64       `tfsdk:"monitor_max_retries_down"`
	MonitorHTTPMethod     types.String      `tfsdk:"monitor_http_method"`
	MonitorExpectedCode   types.Int64       `tfsdk:"monitor_expected_code"`
	MonitorURLPath        types.String      `tfsdk:"monitor_url_path"`
	Listeners             types.List        `tfsdk:"listeners"`
	Pools                 types.List        `tfsdk:"pools"`
	PoolMembers           []PoolMemberInput `tfsdk:"pool_members"`

	AdditionalListeners []AdditionalListenerModel `tfsdk:"additional_listener"`
}

// AdditionalListenerModel is one extra listener added to the load balancer after
// it exists. The API builds it through the same compound-create call as the
// primary listener, but with vttLoadBalancerId set and no loadBalancer object —
// which is how the console attaches a TLS-terminating listener with a cert to a
// running load balancer.
type AdditionalListenerModel struct {
	Protocol      types.String `tfsdk:"protocol"`
	Port          types.Int64  `tfsdk:"port"`
	CertificateID types.String `tfsdk:"certificate_id"`

	PoolAlgorithm          types.String      `tfsdk:"pool_algorithm"`
	PoolSessionPersistence types.String      `tfsdk:"pool_session_persistence"`
	PoolMembers            []PoolMemberInput `tfsdk:"pool_members"`
	MonitorType            types.String      `tfsdk:"monitor_type"`

	// Computed ids, needed to delete the listener, its pool and its monitor.
	ID        types.String `tfsdk:"id"`
	PoolID    types.String `tfsdk:"pool_id"`
	MonitorID types.String `tfsdk:"monitor_id"`
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

// listenerObjectTypes / poolObjectTypes are the element types of the computed
// listeners / pools lists, shared by the schema and the error-path null lists.
func listenerObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":                types.StringType,
		"name":              types.StringType,
		"description":       types.StringType,
		"protocol":          types.StringType,
		"protocol_port":     types.Int64Type,
		"x_forwarded_for":   types.BoolType,
		"x_forwarded_port":  types.BoolType,
		"x_forwarded_proto": types.BoolType,
	}
}

func poolObjectTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":                       types.StringType,
		"name":                     types.StringType,
		"description":              types.StringType,
		"algorithm":                types.StringType,
		"session_persistence_type": types.StringType,
	}
}

func (r *LoadBalancerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_load_balancer"
}

func (r *LoadBalancerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	listenerAttrTypes := listenerObjectTypes()
	poolAttrTypes := poolObjectTypes()

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
				Computed:    true,
				Description: "Description of the Load Balancer. The API stores an empty string when unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subnet_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the subnet where the Load Balancer will be placed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"floating_ip_id": schema.StringAttribute{
				Optional: true,
				Description: "ID of the floating IP to assign to the Load Balancer. Read back from " +
					"the detail endpoint so removing it out-of-band shows as drift.",
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
			"ip_address": schema.StringAttribute{
				Computed:    true,
				Description: "Private IP the Load Balancer listens on, assigned at creation.",
			},
			"provisioning_status": schema.StringAttribute{
				Computed:    true,
				Description: "Provisioning status of the Load Balancer.",
			},
			"is_public_loadbalancer": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the Load Balancer is reachable from the internet.",
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
			"listener_name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the listener created with the Load Balancer. Defaults to <name>-listener.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"listener_protocol": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Protocol the listener accepts. TCP or UDP for a NETWORK TCP-UDP Load Balancer; " +
					"HTTP, HTTPS or TERMINATED_HTTPS for an APPLICATION HTTP-HTTPS one. Use TERMINATED_HTTPS " +
					"together with certificate_id to terminate TLS at the load balancer. Defaults to HTTP.",
				Validators: []validator.String{
					stringvalidator.OneOf("TCP", "UDP", "HTTP", "HTTPS", "TERMINATED_HTTPS"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"certificate_id": schema.StringAttribute{
				Optional: true,
				Description: "Certificate to terminate TLS with, from viettelidc_ovpc_certificate. " +
					"Required when listener_protocol is TERMINATED_HTTPS and only valid then. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"listener_port": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Port the listener accepts traffic on. Defaults to 80.",
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pool_name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the pool created with the Load Balancer. Defaults to <name>-pool.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_algorithm": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "How traffic is spread across members: ROUND_ROBIN, LEAST_CONNECTIONS or SOURCE_IP. Defaults to ROUND_ROBIN.",
				Validators: []validator.String{
					stringvalidator.OneOf("ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_session_persistence": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Pin a client to one member: NONE or SOURCE_IP. Defaults to NONE.",
				Validators: []validator.String{
					stringvalidator.OneOf("NONE", "SOURCE_IP"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"monitor_name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the health monitor. Defaults to <name>-health.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"monitor_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Health check type: HTTP, HTTPS, PING, TCP, TLS-HELLO or UDP-CONNECT. Not every check works with every listener_protocol — the API answers LOADBALANCER_MONITOR_AND_POOL_NOT_VALID_PROTOCOL for pairs it rejects, and which pairs those are is not documented. Defaults to a check derived from listener_protocol.",
				Validators: []validator.String{
					stringvalidator.OneOf("HTTP", "HTTPS", "PING", "TCP", "TLS-HELLO", "UDP-CONNECT"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"monitor_delay": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Seconds between checks. Defaults to 5.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"monitor_timeout": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Seconds before a check times out. Defaults to 5.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"monitor_max_retries": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Successful checks before a member is put back in rotation. Defaults to 3.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"monitor_max_retries_down": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Failed checks before a member is taken out of rotation. Defaults to 3.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"monitor_http_method": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "HTTP method the check sends: GET, HEAD, POST, PUT, DELETE, OPTIONS, PATCH, TRACE or CONNECT. Only used when monitor_type is HTTP or HTTPS. Defaults to GET.",
				Validators: []validator.String{
					stringvalidator.OneOf("GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS", "PATCH", "TRACE", "CONNECT"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"monitor_expected_code": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Status code that counts as healthy. Only used when monitor_type is HTTP or HTTPS. Defaults to 200.",
				Validators: []validator.Int64{
					int64validator.Between(100, 599),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"monitor_url_path": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Path the check requests. Only used when monitor_type is HTTP or HTTPS. Defaults to /.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
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
		Blocks: map[string]schema.Block{
			"additional_listener": schema.ListNestedBlock{
				Description: "Extra listeners added to the load balancer after it is created — this is " +
					"how a TLS-terminating (TERMINATED_HTTPS) listener with a certificate is attached to " +
					"a running load balancer, the same way the console's \"attach certificate\" flow does. " +
					"Each block is its own listener with its own pool, members and health monitor.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"protocol": schema.StringAttribute{
							Required: true,
							Description: "Listener protocol: TCP, UDP, HTTP, HTTPS or TERMINATED_HTTPS. Use " +
								"TERMINATED_HTTPS with certificate_id to terminate TLS.",
							Validators: []validator.String{
								stringvalidator.OneOf("TCP", "UDP", "HTTP", "HTTPS", "TERMINATED_HTTPS"),
							},
						},
						"port": schema.Int64Attribute{
							Required:    true,
							Description: "Port the listener accepts traffic on.",
							Validators:  []validator.Int64{int64validator.Between(1, 65535)},
						},
						"certificate_id": schema.StringAttribute{
							Optional:    true,
							Description: "Certificate to terminate TLS with. Required when protocol is TERMINATED_HTTPS.",
						},
						"pool_algorithm": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "ROUND_ROBIN, LEAST_CONNECTIONS or SOURCE_IP. Defaults to ROUND_ROBIN.",
							Validators: []validator.String{
								stringvalidator.OneOf("ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP"),
							},
						},
						"pool_session_persistence": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "NONE or SOURCE_IP. Defaults to NONE.",
							Validators:  []validator.String{stringvalidator.OneOf("NONE", "SOURCE_IP")},
						},
						"monitor_type": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Health check type. Defaults to a check derived from protocol.",
							Validators: []validator.String{
								stringvalidator.OneOf("HTTP", "HTTPS", "PING", "TCP", "TLS-HELLO", "UDP-CONNECT"),
							},
						},
						"id":         schema.StringAttribute{Computed: true, Description: "Listener ID."},
						"pool_id":    schema.StringAttribute{Computed: true, Description: "Pool ID of this listener."},
						"monitor_id": schema.StringAttribute{Computed: true, Description: "Health monitor ID of this listener."},
					},
					Blocks: map[string]schema.Block{
						"pool_members": schema.ListNestedBlock{
							Description: "Backend VMs for this listener's pool.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"vm_id":  schema.StringAttribute{Required: true, Description: "VM ID to add as a pool member."},
									"port":   schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(80), Description: "Port on the VM (default 80)."},
									"weight": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), Description: "Weight (default 1)."},
								},
							},
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
	// The console lets every one of these be chosen; the provider used to nail
	// them to HTTP/80 round-robin, so a NETWORK TCP-UDP load balancer still got
	// an HTTP listener. Unset falls back to the old values so existing configs
	// keep the load balancer they already have.
	listenerName := defaultStr(plan.ListenerName, plan.Name.ValueString()+"-listener")
	listenerProtocol := defaultStr(plan.ListenerProtocol, "HTTP")
	listenerPort := defaultInt(plan.ListenerPort, 80)
	poolName := defaultStr(plan.PoolName, plan.Name.ValueString()+"-pool")
	poolAlgorithm := defaultStr(plan.PoolAlgorithm, "ROUND_ROBIN")
	poolSessionPersistence := defaultStr(plan.PoolSessionPersistence, "NONE")

	plan.ListenerName = types.StringValue(listenerName)
	plan.ListenerProtocol = types.StringValue(listenerProtocol)
	plan.ListenerPort = types.Int64Value(listenerPort)
	plan.PoolName = types.StringValue(poolName)
	plan.PoolAlgorithm = types.StringValue(poolAlgorithm)
	plan.PoolSessionPersistence = types.StringValue(poolSessionPersistence)

	listenerBody := map[string]interface{}{
		"name":            listenerName,
		"protocol":        listenerProtocol,
		"protocolPort":    listenerPort,
		"xForwardedFor":   false,
		"xForwardedPort":  false,
		"xForwardedProto": false,
	}
	// TERMINATED_HTTPS terminates TLS at the load balancer, which needs the cert
	// to present. This is the create-time attachment the console does through
	// compound-create; changing a live listener's protocol is a separate flow.
	if v := plan.CertificateID.ValueString(); v != "" {
		listenerBody["defaultCertificateId"] = v
	}

	// The monitor has to speak a protocol the listener can carry; a UDP listener
	// with the old hardcoded HTTP check is rejected outright with
	// LOADBALANCER_MONITOR_AND_POOL_NOT_VALID_PROTOCOL. Default it from the
	// listener rather than making every config spell it out.
	monitorName := defaultStr(plan.MonitorName, plan.Name.ValueString()+"-health")
	monitorType := defaultStr(plan.MonitorType, defaultMonitorType(listenerProtocol))
	monitorDelay := defaultInt(plan.MonitorDelay, 5)
	monitorTimeout := defaultInt(plan.MonitorTimeout, 5)
	monitorMaxRetries := defaultInt(plan.MonitorMaxRetries, 3)
	monitorMaxRetriesDown := defaultInt(plan.MonitorMaxRetriesDown, 3)
	monitorHTTPMethod := defaultStr(plan.MonitorHTTPMethod, "GET")
	monitorExpectedCode := defaultInt(plan.MonitorExpectedCode, 200)
	monitorURLPath := defaultStr(plan.MonitorURLPath, "/")

	monitor := map[string]interface{}{
		"name":           monitorName,
		"type":           monitorType,
		"delay":          monitorDelay,
		"timeout":        monitorTimeout,
		"maxRetries":     monitorMaxRetries,
		"maxRetriesDown": monitorMaxRetriesDown,
	}
	// httpMethod, expectedCode and urlPath only mean anything to an HTTP check.
	if isHTTPMonitor(monitorType) {
		monitor["httpMethod"] = monitorHTTPMethod
		monitor["expectedCode"] = monitorExpectedCode
		monitor["urlPath"] = monitorURLPath
	}

	plan.MonitorName = types.StringValue(monitorName)
	plan.MonitorType = types.StringValue(monitorType)
	plan.MonitorDelay = types.Int64Value(monitorDelay)
	plan.MonitorTimeout = types.Int64Value(monitorTimeout)
	plan.MonitorMaxRetries = types.Int64Value(monitorMaxRetries)
	plan.MonitorMaxRetriesDown = types.Int64Value(monitorMaxRetriesDown)
	plan.MonitorHTTPMethod = types.StringValue(monitorHTTPMethod)
	plan.MonitorExpectedCode = types.Int64Value(monitorExpectedCode)
	plan.MonitorURLPath = types.StringValue(monitorURLPath)

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
		"listener": listenerBody,
		"pool": map[string]interface{}{
			"name":                   poolName,
			"algorithm":              poolAlgorithm,
			"sessionPersistenceType": poolSessionPersistence,
			"vpcId":                  parseInt(vpcID),
		},
		"members": members,
		"monitor": monitor,
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

	// Add any extra listeners now that the load balancer exists.
	for i := range plan.AdditionalListeners {
		r.createAdditionalListener(ctx, plan.ID.ValueString(), vpcID, &plan.AdditionalListeners[i], &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Fetch details to get computed fields
	r.readAndMerge(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.readAdditionalListeners(ctx, &plan, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// createAdditionalListener adds one listener (with its pool, members and
// monitor) to an existing load balancer via compound-create — the same call
// that creates a load balancer, minus the loadBalancer object and with
// vttLoadBalancerId set. It then resolves the ids needed to delete it later.
func (r *LoadBalancerResource) createAdditionalListener(ctx context.Context, lbID, vpcID string, l *AdditionalListenerModel, diags *diag.Diagnostics) {
	protocol := l.Protocol.ValueString()
	poolAlgorithm := defaultStr(l.PoolAlgorithm, "ROUND_ROBIN")
	poolSession := defaultStr(l.PoolSessionPersistence, "NONE")
	monitorType := defaultStr(l.MonitorType, defaultMonitorType(protocol))
	name := fmt.Sprintf("%s-%d", protocol, l.Port.ValueInt64())

	members := r.buildMembers(ctx, l.PoolMembers, vpcID, diags)
	if diags.HasError() {
		return
	}

	listenerBody := map[string]interface{}{
		"name":              name,
		"protocol":          protocol,
		"protocolPort":      l.Port.ValueInt64(),
		"xForwardedFor":     false,
		"xForwardedPort":    false,
		"xForwardedProto":   false,
		"vttLoadBalancerId": lbID,
	}
	if v := l.CertificateID.ValueString(); v != "" {
		listenerBody["defaultCertificateId"] = v
	}
	monitor := map[string]interface{}{
		"name": name, "type": monitorType, "delay": 5, "timeout": 5,
		"maxRetries": 3, "maxRetriesDown": 3,
	}
	if isHTTPMonitor(monitorType) {
		monitor["httpMethod"] = "GET"
		monitor["expectedCode"] = 200
		monitor["urlPath"] = "/"
	}
	body := map[string]interface{}{
		"listener": listenerBody,
		"pool": map[string]interface{}{
			"name": name, "algorithm": poolAlgorithm,
			"sessionPersistenceType": poolSession, "vpcId": parseInt(vpcID),
		},
		"members":     members,
		"monitor":     monitor,
		"vpc_id":      vpcID,
		"customer_id": r.customerID,
	}
	if _, d := callAPIRetryBusy(ctx, r.client, pathLoadBalancerCreate, body, asyncOpTimeout); d.HasError() {
		diags.Append(d...)
		return
	}

	l.Protocol = types.StringValue(protocol)
	l.PoolAlgorithm = types.StringValue(poolAlgorithm)
	l.PoolSessionPersistence = types.StringValue(poolSession)
	l.MonitorType = types.StringValue(monitorType)
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
	r.readAdditionalListeners(ctx, &state, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *LoadBalancerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state LoadBalancerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reconcile additional listeners: an additional listener is identified by
	// protocol+port. Ones in the plan but not the prior state are added, ones
	// gone from the plan are deleted (monitor, then pool, then listener).
	key := func(l AdditionalListenerModel) string {
		return strings.ToUpper(l.Protocol.ValueString()) + ":" + fmt.Sprintf("%d", l.Port.ValueInt64())
	}
	planKeys := map[string]bool{}
	for _, l := range plan.AdditionalListeners {
		planKeys[key(l)] = true
	}
	stateByKey := map[string]AdditionalListenerModel{}
	for _, l := range state.AdditionalListeners {
		stateByKey[key(l)] = l
	}
	for _, l := range state.AdditionalListeners {
		if !planKeys[key(l)] {
			r.deleteAdditionalListener(ctx, plan.VpcID.ValueString(), l, &resp.Diagnostics)
		}
	}
	for i := range plan.AdditionalListeners {
		if _, existed := stateByKey[key(plan.AdditionalListeners[i])]; !existed {
			r.createAdditionalListener(ctx, plan.ID.ValueString(), plan.VpcID.ValueString(), &plan.AdditionalListeners[i], &resp.Diagnostics)
		}
	}
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
	r.readAdditionalListeners(ctx, &plan, &resp.Diagnostics)

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

	// The pool can still be settling from create/update, which CSA reports as
	// ERROR_POOL_IS_IN_OTHER_PROCESSING. Wait it out rather than failing the
	// destroy and leaving the LB behind.
	apiResp, diags := callAPIRetryBusy(ctx, r.client, pathLoadBalancerDelete, body, asyncOpTimeout)
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
	if err := pollUntilGone(ctx, r.client, pathLoadBalancerDetail, pollBody, asyncOpTimeout); err != nil {
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
		VttLoadBalancerID    int64  `json:"vttLoadBalancerId"`
		Name                 string `json:"name"`
		Description          string `json:"description"`
		VttSubnetID          int64  `json:"vttSubnetId"`
		LoadBalancerType     string `json:"vttLoadbalancerTypeName"`
		PackageType          string `json:"loadbalancerTypeName"`
		AdminStateUp         bool   `json:"adminStateUp"`
		Status               string `json:"status"`
		OperatingStatus      string `json:"operatingStatus"`
		IPAddress            string `json:"ipAddress"`
		ProvisioningStatus   string `json:"provisioningStatus"`
		IsPublicLoadbalancer bool   `json:"isPublicLoadbalancer"`
		// vttFloatingId is the key compound-create sends for the assigned
		// floating IP; the detail endpoint omits it when none is attached. No
		// captured LB carried one, so this is the create key read back rather
		// than a field confirmed against a FIP-bearing LB.
		VttFloatingID *int64 `json:"vttFloatingId"`
	}

	if err := json.Unmarshal(apiResp.Data, &detailResp); err != nil {
		diags.AddError("Parse Error", err.Error())
		return
	}

	// If the load balancer is in a terminal error state, record the status and
	// stop — Delete() runs regardless of status. Every computed attribute still
	// has to be a known value or the create fails with "invalid result object",
	// so fill the read-only fields with knowns rather than leaving them unknown.
	if st := strings.ToUpper(detailResp.Status); st == "ERROR" || st == "FAILED" {
		model.Status = types.StringValue(detailResp.Status)
		model.IPAddress = types.StringValue(detailResp.IPAddress)
		model.OperatingStatus = types.StringValue(detailResp.OperatingStatus)
		model.ProvisioningStatus = types.StringValue(detailResp.ProvisioningStatus)
		model.IsPublicLoadBalancer = types.BoolValue(detailResp.IsPublicLoadbalancer)
		if model.Listeners.IsUnknown() {
			model.Listeners = types.ListNull(types.ObjectType{AttrTypes: listenerObjectTypes()})
		}
		if model.Pools.IsUnknown() {
			model.Pools = types.ListNull(types.ObjectType{AttrTypes: poolObjectTypes()})
		}
		return
	}

	model.Name = types.StringValue(detailResp.Name)
	model.Description = types.StringValue(detailResp.Description)
	model.SubnetID = types.StringValue(fmt.Sprintf("%d", detailResp.VttSubnetID))
	model.IPAddress = types.StringValue(detailResp.IPAddress)
	model.ProvisioningStatus = types.StringValue(detailResp.ProvisioningStatus)
	model.IsPublicLoadBalancer = types.BoolValue(detailResp.IsPublicLoadbalancer)
	if detailResp.VttFloatingID != nil && *detailResp.VttFloatingID != 0 {
		model.FloatingIPID = types.StringValue(fmt.Sprintf("%d", *detailResp.VttFloatingID))
	}
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

// defaultStr returns v, or fallback when the config left it out.
func defaultStr(v types.String, fallback string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return fallback
	}
	return v.ValueString()
}

func defaultInt(v types.Int64, fallback int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return fallback
	}
	return v.ValueInt64()
}

// ValidateConfig rejects a listener protocol the load balancer type cannot
// serve. The API takes the request either way and builds something unusable.
func (r *LoadBalancerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg LoadBalancerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateListenerProtocol(cfg.LoadBalancerType, cfg.ListenerProtocol)...)
	resp.Diagnostics.Append(validateCertificate(cfg.ListenerProtocol, cfg.CertificateID)...)
	for _, l := range cfg.AdditionalListeners {
		resp.Diagnostics.Append(validateListenerProtocol(cfg.LoadBalancerType, l.Protocol)...)
		resp.Diagnostics.Append(validateCertificate(l.Protocol, l.CertificateID)...)
	}
}

// validateCertificate ties certificate_id to a TERMINATED_HTTPS listener: the
// cert is only used to terminate TLS, and that protocol is the only one that
// does. Both directions matter — a TERMINATED_HTTPS listener with no cert has
// nothing to present.
func validateCertificate(protocol, certID types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	// A cert sourced from another resource is unknown at plan time; we cannot
	// tell whether it is set, so leave the pairing for apply to enforce.
	if protocol.IsUnknown() || certID.IsUnknown() {
		return diags
	}
	term := !protocol.IsNull() && strings.EqualFold(protocol.ValueString(), "TERMINATED_HTTPS")
	hasCert := !certID.IsNull() && certID.ValueString() != ""

	if term && !hasCert {
		diags.AddAttributeError(path.Root("certificate_id"), "Certificate Required",
			"listener_protocol is TERMINATED_HTTPS; set certificate_id to the cert to terminate TLS with.")
	}
	if hasCert && !term {
		diags.AddAttributeError(path.Root("certificate_id"), "Certificate Not Used",
			"certificate_id only applies when listener_protocol is TERMINATED_HTTPS.")
	}
	return diags
}

// validateListenerProtocol is the rule on its own so it can be tested.
func validateListenerProtocol(lbType, protocol types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	if protocol.IsNull() || protocol.IsUnknown() || lbType.IsNull() || lbType.IsUnknown() {
		return diags
	}
	allowed := map[string][]string{
		"NETWORK TCP-UDP":        {"TCP", "UDP"},
		"APPLICATION HTTP-HTTPS": {"HTTP", "HTTPS", "TERMINATED_HTTPS"},
	}
	want, known := allowed[strings.ToUpper(lbType.ValueString())]
	if !known {
		return diags
	}
	got := strings.ToUpper(protocol.ValueString())
	for _, ok := range want {
		if got == ok {
			return diags
		}
	}
	diags.AddAttributeError(path.Root("listener_protocol"), "Protocol Not Supported By This Load Balancer",
		fmt.Sprintf("loadbalancer_type %q serves %s; listener_protocol is %q.",
			lbType.ValueString(), strings.Join(want, " or "), protocol.ValueString()))
	return diags
}

// defaultMonitorType picks a check that plausibly suits the listener. The API
// rejects some combinations with LOADBALANCER_MONITOR_AND_POOL_NOT_VALID_PROTOCOL
// and the exact rule is not documented anywhere we can see, so this is only a
// better starting point than the old hardcoded HTTP — set monitor_type
// explicitly when the API disagrees.
func defaultMonitorType(listenerProtocol string) string {
	switch strings.ToUpper(listenerProtocol) {
	case "UDP":
		return "UDP-CONNECT"
	case "TCP":
		return "TCP"
	case "HTTPS":
		return "HTTPS"
	default:
		return "HTTP"
	}
}

func isHTTPMonitor(monitorType string) bool {
	t := strings.ToUpper(monitorType)
	return t == "HTTP" || t == "HTTPS"
}

// readAdditionalListeners resolves the id, pool_id and monitor_id of each
// additional listener by correlating the three list endpoints:
// a listener is matched by protocol+port (unique on a load balancer), its pool
// by vttLoadBalancerListenerId, and its monitor by vttLoadBalancerPoolId.
func (r *LoadBalancerResource) readAdditionalListeners(ctx context.Context, m *LoadBalancerResourceModel, diags *diag.Diagnostics) {
	if len(m.AdditionalListeners) == 0 {
		return
	}
	body := map[string]interface{}{
		"vpc_id": m.VpcID.ValueString(), "customer_id": r.customerID,
		"vttLoadBalancerId": parseInt(m.ID.ValueString()),
	}

	lResp, d := callAPI(ctx, r.client, pathLoadBalancerListeners, body)
	diags.Append(d...)
	pResp, d := callAPI(ctx, r.client, pathLoadBalancerPools, body)
	diags.Append(d...)
	mResp, d := callAPI(ctx, r.client, pathLoadBalancerMonitors, body)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	var listeners []struct {
		ID           string `json:"id"`
		Protocol     string `json:"protocol"`
		ProtocolPort int64  `json:"protocolPort"`
	}
	var pools []struct {
		ID         string `json:"id"`
		ListenerID int64  `json:"vttLoadBalancerListenerId"`
	}
	var monitors []struct {
		ID     string `json:"id"`
		PoolID int64  `json:"vttLoadBalancerPoolId"`
	}
	_ = json.Unmarshal(lResp.Data, &listeners)
	_ = json.Unmarshal(pResp.Data, &pools)
	_ = json.Unmarshal(mResp.Data, &monitors)

	for i := range m.AdditionalListeners {
		l := &m.AdditionalListeners[i]
		var listenerID string
		for _, x := range listeners {
			if strings.EqualFold(x.Protocol, l.Protocol.ValueString()) && x.ProtocolPort == l.Port.ValueInt64() {
				listenerID = x.ID
				break
			}
		}
		l.ID = types.StringValue(listenerID)

		var poolID string
		for _, p := range pools {
			if fmt.Sprintf("%d", p.ListenerID) == listenerID {
				poolID = p.ID
				break
			}
		}
		l.PoolID = types.StringValue(poolID)

		var monitorID string
		for _, mo := range monitors {
			if fmt.Sprintf("%d", mo.PoolID) == poolID {
				monitorID = mo.ID
				break
			}
		}
		l.MonitorID = types.StringValue(monitorID)
	}
}

// deleteAdditionalListener removes a listener and everything created with it:
// the monitor and the pool (which cascades its members), then the listener.
func (r *LoadBalancerResource) deleteAdditionalListener(ctx context.Context, vpcID string, l AdditionalListenerModel, diags *diag.Diagnostics) {
	del := func(path, key, id string) {
		if id == "" {
			return
		}
		body := map[string]interface{}{key: id, "vpc_id": vpcID, "customer_id": r.customerID}
		apiResp, d := callAPIRetryBusy(ctx, r.client, path, body, asyncOpTimeout)
		if d.HasError() {
			// pool/delete cascades the pool's members and its monitor, so by the
			// time monitor/delete runs the monitor is already gone. Treat a
			// not-found as success rather than failing the whole reconcile.
			if apiResp != nil && isNotFoundMessage(apiResp.Message) {
				return
			}
			diags.Append(d...)
		}
	}
	// Order matters and is top-down: the listener references the pool and the
	// pool the monitor, so each must go before the thing it points at is gone.
	// Deleting the monitor or pool while the listener still references them
	// returns success without actually removing them — that is how a reversed
	// order leaves orphans behind.
	del(pathLoadBalancerListenerDelete, "vttLoadBalancerListenerId", l.ID.ValueString())
	del(pathLoadBalancerPoolDelete, "vttLoadBalancerPoolId", l.PoolID.ValueString())
	del(pathLoadBalancerMonitorDelete, "vttLoadBalancerHealthMonitorId", l.MonitorID.ValueString())
}
