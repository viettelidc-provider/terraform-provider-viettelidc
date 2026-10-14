// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*L7PoliciesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*L7PoliciesDataSource)(nil)
)

type L7PoliciesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7PoliciesDataSource() datasource.DataSource {
	return &L7PoliciesDataSource{}
}

type L7PoliciesDataSourceModel struct {
	ID         types.String   `tfsdk:"id"`
	VpcID      types.Int64    `tfsdk:"vpc_id"`
	ListenerID types.Int64    `tfsdk:"listener_id"`
	Policies   []L7PolicyItem `tfsdk:"policies"`
}

type L7PolicyItem struct {
	ID             types.String `tfsdk:"id"`
	PolicyID       types.Int64  `tfsdk:"policy_id"`
	ListenerID     types.Int64  `tfsdk:"listener_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Action         types.String `tfsdk:"action"`
	RedirectURL    types.String `tfsdk:"redirect_url"`
	RedirectPoolID types.Int64  `tfsdk:"redirect_pool_id"`
	PoolName       types.String `tfsdk:"pool_name"`
	Priority       types.Int64  `tfsdk:"priority"`
	Status         types.String `tfsdk:"status"`
	NumberOfRules  types.Int64  `tfsdk:"number_of_rules"`
}

func (d *L7PoliciesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_policies"
}

func (d *L7PoliciesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing ViettelIDC vLB Layer 7 Policies for a listener.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier for this data source reading (<vpc_id>/<listener_id>).",
			},
			"vpc_id": schema.Int64Attribute{
				Required:    true,
				Description: "VPC ID owning the Load Balancer.",
			},
			"listener_id": schema.Int64Attribute{
				Required:    true,
				Description: "Listener ID whose L7 policies are listed.",
			},
			"policies": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of L7 Policies.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Policy ID as string.",
						},
						"policy_id": schema.Int64Attribute{
							Computed:    true,
							Description: "CMP internal policy ID.",
						},
						"listener_id": schema.Int64Attribute{
							Computed:    true,
							Description: "Listener ID.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Policy name.",
						},
						"description": schema.StringAttribute{
							Computed:    true,
							Description: "Policy description.",
						},
						"action": schema.StringAttribute{
							Computed:    true,
							Description: "Policy action: REJECT, REDIRECT_TO_URL, or REDIRECT_TO_POOL.",
						},
						"redirect_url": schema.StringAttribute{
							Computed:    true,
							Description: "Redirect URL if action is REDIRECT_TO_URL.",
						},
						"redirect_pool_id": schema.Int64Attribute{
							Computed:    true,
							Description: "Redirect Pool ID if action is REDIRECT_TO_POOL.",
						},
						"pool_name": schema.StringAttribute{
							Computed:    true,
							Description: "Redirect Pool Name if action is REDIRECT_TO_POOL.",
						},
						"priority": schema.Int64Attribute{
							Computed:    true,
							Description: "Policy priority / position.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Lifecycle status of the policy.",
						},
						"number_of_rules": schema.Int64Attribute{
							Computed:    true,
							Description: "Number of rules configured in this policy.",
						},
					},
				},
			},
		},
	}
}

func (d *L7PoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *L7PoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg L7PoliciesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := cfg.VpcID.ValueInt64()
	listenerID := cfg.ListenerID.ValueInt64()

	body := map[string]interface{}{
		"vpcId":                     vpcID,
		"vpc_id":                    vpcID,
		"customerId":                d.customerID,
		"customer_id":               d.customerID,
		"vttLoadBalancerListenerId": listenerID,
		"listener_id":               listenerID,
		"pageIndex":                 0,
		"pageSize":                  1000,
	}

	apiResp, diags := callAPI(ctx, d.client, pathL7PolicyList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listEnvelope struct {
		PageIndex  int                      `json:"pageIndex"`
		PageSize   int                      `json:"pageSize"`
		TotalItems int                      `json:"totalItems"`
		Items      []l7PolicyDetailResponse `json:"items"`
	}

	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &listEnvelope); err != nil {
			resp.Diagnostics.AddError("Parse Policy List Failed", err.Error())
			return
		}
	}

	policies := make([]L7PolicyItem, 0, len(listEnvelope.Items))
	for _, item := range listEnvelope.Items {
		pID := item.VttLoadBalancerL7PolicyId
		if pID == 0 && item.ID != nil {
			pID = parseID(item.ID)
		}

		p := L7PolicyItem{
			ID:            types.StringValue(strconv.FormatInt(pID, 10)),
			PolicyID:      types.Int64Value(pID),
			ListenerID:    types.Int64Value(item.VttLoadBalancerListenerId),
			Name:          types.StringValue(item.Name),
			Action:        types.StringValue(normalizeAction(item.Action)),
			Status:        types.StringValue(strings.ToLower(item.Status)),
			NumberOfRules: types.Int64Value(item.NumberOfRules),
		}

		if item.Description != "" {
			p.Description = types.StringValue(item.Description)
		} else {
			p.Description = types.StringNull()
		}

		if item.RedirectURL != "" {
			p.RedirectURL = types.StringValue(item.RedirectURL)
		} else {
			p.RedirectURL = types.StringNull()
		}

		if item.VttLoadBalancerPoolId > 0 {
			p.RedirectPoolID = types.Int64Value(item.VttLoadBalancerPoolId)
		} else {
			p.RedirectPoolID = types.Int64Null()
		}

		if item.PoolName != "" {
			p.PoolName = types.StringValue(item.PoolName)
		} else {
			p.PoolName = types.StringNull()
		}

		if item.Priority != nil {
			p.Priority = types.Int64Value(*item.Priority)
		} else {
			p.Priority = types.Int64Null()
		}

		policies = append(policies, p)
	}

	cfg.ID = types.StringValue(fmt.Sprintf("%d/%d", vpcID, listenerID))
	cfg.Policies = policies
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
