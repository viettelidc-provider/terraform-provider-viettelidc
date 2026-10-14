// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*LBListenerDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*LBListenerDataSource)(nil)
)

type LBListenerDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewLBListenerDataSource() datasource.DataSource {
	return &LBListenerDataSource{}
}

type LBListenerDataSourceModel struct {
	VpcID            types.Int64  `tfsdk:"vpc_id"`
	LoadBalancerID   types.Int64  `tfsdk:"load_balancer_id"`
	LoadBalancerName types.String `tfsdk:"load_balancer_name"`
	ID               types.Int64  `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Port           types.Int64  `tfsdk:"port"`
	Protocol       types.String `tfsdk:"protocol"`
	Status         types.String `tfsdk:"status"`
	DefaultPoolID  types.Int64  `tfsdk:"default_pool_id"`
}

func (d *LBListenerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_listener"
}

func (d *LBListenerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single Load Balancer listener by name, port, or ID.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.Int64Attribute{
				Description: "VPC ID.",
				Required:    true,
			},
			"load_balancer_id": schema.Int64Attribute{
				Description: "Load Balancer ID. At least one of 'load_balancer_id' or 'load_balancer_name' must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"load_balancer_name": schema.StringAttribute{
				Description: "Load Balancer name. If set, looks up the load balancer by name in the VPC to retrieve its ID automatically.",
				Optional:    true,
				Computed:    true,
			},
			"id": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Listener name.",
				Optional:    true,
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "Listener protocol port.",
				Optional:    true,
				Computed:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "Listener protocol (HTTP, HTTPS, TCP, etc.).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Listener status.",
				Computed:    true,
			},
			"default_pool_id": schema.Int64Attribute{
				Description: "Default backend pool ID associated with the listener.",
				Computed:    true,
			},
		},
	}
}

func (d *LBListenerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LBListenerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg LBListenerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, diags := d.lookup(ctx, &cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, res)...)
}

func (d *LBListenerDataSource) lookup(ctx context.Context, cfg *LBListenerDataSourceModel) (*LBListenerDataSourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	hasID := !cfg.ID.IsNull() && !cfg.ID.IsUnknown()
	hasName := !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != ""
	hasPort := !cfg.Port.IsNull() && !cfg.Port.IsUnknown()

	if !hasID && !hasName && !hasPort {
		diags.AddError("Missing lookup filter", "At least one of 'id', 'name', or 'port' must be specified.")
		return nil, diags
	}

	hasLBID := !cfg.LoadBalancerID.IsNull() && !cfg.LoadBalancerID.IsUnknown()
	hasLBName := !cfg.LoadBalancerName.IsNull() && !cfg.LoadBalancerName.IsUnknown() && cfg.LoadBalancerName.ValueString() != ""

	if !hasLBID && !hasLBName {
		diags.AddError("Missing load balancer identifier", "At least one of 'load_balancer_id' or 'load_balancer_name' must be specified.")
		return nil, diags
	}

	vpcID := cfg.VpcID.ValueInt64()
	lbID := cfg.LoadBalancerID.ValueInt64()

	if hasLBName {
		resolvedID, resDiags := resolveLoadBalancerIDByName(ctx, d.client, d.customerID, vpcID, cfg.LoadBalancerName.ValueString())
		diags.Append(resDiags...)
		if diags.HasError() {
			return nil, diags
		}
		if hasLBID && lbID != resolvedID {
			diags.AddError(
				"Conflicting Load Balancer Identifier",
				fmt.Sprintf("load_balancer_name %q resolved to ID %d, but load_balancer_id is %d", cfg.LoadBalancerName.ValueString(), resolvedID, lbID),
			)
			return nil, diags
		}
		lbID = resolvedID
	}
	body := map[string]interface{}{
		"vpc_id":            vpcID,
		"vpcId":             vpcID,
		"customer_id":       d.customerID,
		"customerId":        d.customerID,
		"vttLoadBalancerId": lbID,
	}

	apiResp, dDiags := callAPI(ctx, d.client, pathLoadBalancerListeners, body)
	diags.Append(dDiags...)
	if diags.HasError() {
		return nil, diags
	}

	type rawListenerItem struct {
		ID                        interface{} `json:"id"`
		VttLoadBalancerListenerId int64       `json:"vttLoadBalancerListenerId"`
		Name                      string      `json:"name"`
		Protocol                  string      `json:"protocol"`
		ProtocolPort              int64       `json:"protocolPort"`
		Status                    string      `json:"status"`
		DefaultPoolID             interface{} `json:"defaultPoolId"`
		VttLoadBalancerPoolId     interface{} `json:"vttLoadBalancerPoolId"`
	}

	var listeners []rawListenerItem
	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &listeners); err != nil {
			var envelope struct {
				Items []rawListenerItem `json:"items"`
			}
			if err2 := json.Unmarshal(apiResp.Data, &envelope); err2 == nil {
				listeners = envelope.Items
			} else {
				diags.AddError("Parse Listeners Failed", err.Error())
				return nil, diags
			}
		}
	}

	var matched []rawListenerItem
	for _, l := range listeners {
		lID := l.VttLoadBalancerListenerId
		if lID == 0 && l.ID != nil {
			lID = parseID(l.ID)
		}
		l.VttLoadBalancerListenerId = lID

		if hasID && lID != cfg.ID.ValueInt64() {
			continue
		}
		if hasName && l.Name != cfg.Name.ValueString() {
			continue
		}
		if hasPort && l.ProtocolPort != cfg.Port.ValueInt64() {
			continue
		}
		matched = append(matched, l)
	}

	if len(matched) == 0 {
		diags.AddError(
			"Listener Not Found",
			fmt.Sprintf("No listener found on load balancer %d matching criteria (id=%v, name=%v, port=%v)", lbID, cfg.ID, cfg.Name, cfg.Port),
		)
		return nil, diags
	}
	if len(matched) > 1 {
		diags.AddError(
			"Multiple Listeners Found",
			fmt.Sprintf("Found %d listeners on load balancer %d matching criteria; please specify a more specific filter", len(matched), lbID),
		)
		return nil, diags
	}

	found := matched[0]

	status := strings.ToLower(found.Status)
	if status == "" {
		detailBody := map[string]interface{}{
			"vpc_id":                    vpcID,
			"customer_id":               d.customerID,
			"vttLoadBalancerListenerId": found.VttLoadBalancerListenerId,
			"id":                        found.VttLoadBalancerListenerId,
		}
		if dResp, d2 := callAPI(ctx, d.client, pathLoadBalancerListenerDetail, detailBody); !d2.HasError() && dResp != nil && len(dResp.Data) > 0 {
			var dObj struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(dResp.Data, &dObj); err == nil && dObj.Status != "" {
				status = strings.ToLower(dObj.Status)
			}
		}
	}
	if status == "" {
		status = "active"
	}

	var poolID int64
	if found.DefaultPoolID != nil {
		poolID = parseID(found.DefaultPoolID)
	}
	if poolID == 0 && found.VttLoadBalancerPoolId != nil {
		poolID = parseID(found.VttLoadBalancerPoolId)
	}
	if poolID == 0 {
		poolBody := map[string]interface{}{
			"vpc_id":            vpcID,
			"customer_id":       d.customerID,
			"vttLoadBalancerId": lbID,
		}
		if pResp, pDiags := callAPI(ctx, d.client, pathLoadBalancerPools, poolBody); !pDiags.HasError() && pResp != nil && len(pResp.Data) > 0 {
			type poolItem struct {
				ID                    interface{} `json:"id"`
				VttLoadBalancerPoolId int64       `json:"vttLoadBalancerPoolId"`
				ListenerID            int64       `json:"vttLoadBalancerListenerId"`
			}
			var pools []poolItem
			if err := json.Unmarshal(pResp.Data, &pools); err != nil {
				var envelope struct {
					Items []poolItem `json:"items"`
				}
				if err2 := json.Unmarshal(pResp.Data, &envelope); err2 == nil {
					pools = envelope.Items
				}
			}
			for _, p := range pools {
				if p.ListenerID == found.VttLoadBalancerListenerId {
					pID := p.VttLoadBalancerPoolId
					if pID == 0 && p.ID != nil {
						pID = parseID(p.ID)
					}
					poolID = pID
					break
				}
			}
		}
	}

	res := &LBListenerDataSourceModel{
		VpcID:            types.Int64Value(vpcID),
		LoadBalancerID:   types.Int64Value(lbID),
		LoadBalancerName: cfg.LoadBalancerName,
		ID:               types.Int64Value(found.VttLoadBalancerListenerId),
		Name:           types.StringValue(found.Name),
		Port:           types.Int64Value(found.ProtocolPort),
		Protocol:       types.StringValue(found.Protocol),
		Status:         types.StringValue(status),
	}
	if poolID > 0 {
		res.DefaultPoolID = types.Int64Value(poolID)
	} else {
		res.DefaultPoolID = types.Int64Null()
	}

	return res, diags
}
