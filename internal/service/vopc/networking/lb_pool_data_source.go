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
	_ datasource.DataSource              = (*LBPoolDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*LBPoolDataSource)(nil)
)

type LBPoolDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewLBPoolDataSource() datasource.DataSource {
	return &LBPoolDataSource{}
}

type LBPoolDataSourceModel struct {
	VpcID            types.Int64  `tfsdk:"vpc_id"`
	LoadBalancerID   types.Int64  `tfsdk:"load_balancer_id"`
	LoadBalancerName types.String `tfsdk:"load_balancer_name"`
	ID               types.Int64  `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Protocol       types.String `tfsdk:"protocol"`
	Algorithm      types.String `tfsdk:"algorithm"`
	Status         types.String `tfsdk:"status"`
}

func (d *LBPoolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_pool"
}

func (d *LBPoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single Load Balancer backend pool by name or ID.",
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
				Description: "Pool name.",
				Optional:    true,
				Computed:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "Pool protocol (HTTP, TCP, etc.).",
				Computed:    true,
			},
			"algorithm": schema.StringAttribute{
				Description: "Load balancing algorithm (ROUND_ROBIN, LEAST_CONNECTIONS, SOURCE_IP).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Pool status.",
				Computed:    true,
			},
		},
	}
}

func (d *LBPoolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *LBPoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg LBPoolDataSourceModel
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

func (d *LBPoolDataSource) lookup(ctx context.Context, cfg *LBPoolDataSourceModel) (*LBPoolDataSourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	hasID := !cfg.ID.IsNull() && !cfg.ID.IsUnknown()
	hasName := !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != ""

	if !hasID && !hasName {
		diags.AddError("Missing lookup filter", "At least one of 'id' or 'name' must be specified.")
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

	apiResp, dDiags := callAPI(ctx, d.client, pathLoadBalancerPools, body)
	diags.Append(dDiags...)
	if diags.HasError() {
		return nil, diags
	}

	type rawPoolItem struct {
		ID                    interface{} `json:"id"`
		VttLoadBalancerPoolId int64       `json:"vttLoadBalancerPoolId"`
		VttLoadBalancerId     int64       `json:"vttLoadBalancerId"`
		Name                  string      `json:"name"`
		Description           string      `json:"description"`
		Protocol              string      `json:"protocol"`
		Algorithm             string      `json:"algorithm"`
		Status                string      `json:"status"`
	}

	var pools []rawPoolItem
	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &pools); err != nil {
			var envelope struct {
				Items []rawPoolItem `json:"items"`
			}
			if err2 := json.Unmarshal(apiResp.Data, &envelope); err2 == nil {
				pools = envelope.Items
			} else {
				diags.AddError("Parse Pools Failed", err.Error())
				return nil, diags
			}
		}
	}

	var matched []rawPoolItem
	for _, p := range pools {
		pID := p.VttLoadBalancerPoolId
		if pID == 0 && p.ID != nil {
			pID = parseID(p.ID)
		}
		p.VttLoadBalancerPoolId = pID

		if hasID && pID != cfg.ID.ValueInt64() {
			continue
		}
		if hasName && p.Name != cfg.Name.ValueString() {
			continue
		}
		matched = append(matched, p)
	}

	if len(matched) == 0 {
		diags.AddError(
			"Pool Not Found",
			fmt.Sprintf("No pool found on load balancer %d matching criteria (id=%v, name=%v)", lbID, cfg.ID, cfg.Name),
		)
		return nil, diags
	}
	if len(matched) > 1 {
		diags.AddError(
			"Multiple Pools Found",
			fmt.Sprintf("Found %d pools on load balancer %d matching criteria; please specify a more specific filter", len(matched), lbID),
		)
		return nil, diags
	}

	found := matched[0]
	protocol := found.Protocol
	status := strings.ToLower(found.Status)

	if protocol == "" || status == "" {
		detailBody := map[string]interface{}{
			"vpc_id":                vpcID,
			"customer_id":           d.customerID,
			"vttLoadBalancerPoolId": found.VttLoadBalancerPoolId,
			"id":                    found.VttLoadBalancerPoolId,
		}
		if dResp, d2 := callAPI(ctx, d.client, pathLoadBalancerPoolDetail, detailBody); !d2.HasError() && dResp != nil && len(dResp.Data) > 0 {
			var dObj struct {
				Protocol string `json:"protocol"`
				Status   string `json:"status"`
			}
			if err := json.Unmarshal(dResp.Data, &dObj); err == nil {
				if protocol == "" && dObj.Protocol != "" {
					protocol = dObj.Protocol
				}
				if status == "" && dObj.Status != "" {
					status = strings.ToLower(dObj.Status)
				}
			}
		}
	}

	if status == "" {
		status = "active"
	}

	res := &LBPoolDataSourceModel{
		VpcID:            types.Int64Value(vpcID),
		LoadBalancerID:   types.Int64Value(lbID),
		LoadBalancerName: cfg.LoadBalancerName,
		ID:               types.Int64Value(found.VttLoadBalancerPoolId),
		Name:           types.StringValue(found.Name),
		Algorithm:      types.StringValue(found.Algorithm),
		Protocol:       types.StringValue(protocol),
		Status:         types.StringValue(status),
	}

	return res, diags
}
