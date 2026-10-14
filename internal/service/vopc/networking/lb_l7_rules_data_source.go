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
	_ datasource.DataSource              = (*L7RulesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*L7RulesDataSource)(nil)
)

type L7RulesDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7RulesDataSource() datasource.DataSource {
	return &L7RulesDataSource{}
}

type L7RulesDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	VpcID      types.Int64  `tfsdk:"vpc_id"`
	ListenerID types.Int64  `tfsdk:"listener_id"`
	PolicyID   types.Int64  `tfsdk:"policy_id"`
	Rules      []L7RuleItem `tfsdk:"rules"`
}

type L7RuleItem struct {
	ID          types.String `tfsdk:"id"`
	RuleID      types.Int64  `tfsdk:"rule_id"`
	PolicyID    types.Int64  `tfsdk:"policy_id"`
	ListenerID  types.Int64  `tfsdk:"listener_id"`
	Type        types.String `tfsdk:"type"`
	CompareType types.String `tfsdk:"compare_type"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Status      types.String `tfsdk:"status"`
}

func (d *L7RulesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_rules"
}

func (d *L7RulesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing ViettelIDC vLB Layer 7 Rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier for this data source reading (<vpc_id>/<listener_id>[/<policy_id>]).",
			},
			"vpc_id": schema.Int64Attribute{
				Required:    true,
				Description: "VPC ID owning the Load Balancer.",
			},
			"listener_id": schema.Int64Attribute{
				Required:    true,
				Description: "Listener ID whose L7 rules are listed.",
			},
			"policy_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Optional L7 Policy ID filter.",
			},
			"rules": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of L7 Rules.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID as string.",
						},
						"rule_id": schema.Int64Attribute{
							Computed:    true,
							Description: "CMP internal rule ID.",
						},
						"policy_id": schema.Int64Attribute{
							Computed:    true,
							Description: "Parent policy ID.",
						},
						"listener_id": schema.Int64Attribute{
							Computed:    true,
							Description: "Listener ID.",
						},
						"type": schema.StringAttribute{
							Computed:    true,
							Description: "Rule type (HOST_NAME, PATH, FILE_TYPE, HEADER, COOKIE).",
						},
						"compare_type": schema.StringAttribute{
							Computed:    true,
							Description: "Comparison type (EQUAL_TO, STARTS_WITH, ENDS_WITH, CONTAINS, REGEX).",
						},
						"key": schema.StringAttribute{
							Computed:    true,
							Description: "Header or cookie name if applicable.",
						},
						"value": schema.StringAttribute{
							Computed:    true,
							Description: "Comparison value.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Lifecycle status of the rule.",
						},
					},
				},
			},
		},
	}
}

func (d *L7RulesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *L7RulesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg L7RulesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpcID := cfg.VpcID.ValueInt64()
	listenerID := cfg.ListenerID.ValueInt64()

	// In list: NEVER send vttLoadBalancerL7RuleId
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

	var hasPolicyFilter bool
	var filterPolicyID int64
	if !cfg.PolicyID.IsNull() && !cfg.PolicyID.IsUnknown() && cfg.PolicyID.ValueInt64() > 0 {
		hasPolicyFilter = true
		filterPolicyID = cfg.PolicyID.ValueInt64()
		body["vttLoadBalancerL7PolicyId"] = filterPolicyID
		body["policy_id"] = filterPolicyID
	}

	apiResp, diags := callAPI(ctx, d.client, pathL7RuleList, body)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var listEnvelope struct {
		PageIndex  int                    `json:"pageIndex"`
		PageSize   int                    `json:"pageSize"`
		TotalItems int                    `json:"totalItems"`
		Items      []l7RuleDetailResponse `json:"items"`
	}

	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &listEnvelope); err != nil {
			resp.Diagnostics.AddError("Parse Rule List Failed", err.Error())
			return
		}
	}

	rules := make([]L7RuleItem, 0, len(listEnvelope.Items))
	for _, item := range listEnvelope.Items {
		rID := item.VttLoadBalancerL7RuleId
		if rID == 0 && item.ID != nil {
			rID = parseID(item.ID)
		}

		if hasPolicyFilter && item.VttLoadBalancerL7PolicyId != filterPolicyID {
			continue
		}

		ruleItem := L7RuleItem{
			ID:          types.StringValue(strconv.FormatInt(rID, 10)),
			RuleID:      types.Int64Value(rID),
			PolicyID:    types.Int64Value(item.VttLoadBalancerL7PolicyId),
			ListenerID:  types.Int64Value(item.VttLoadBalancerListenerId),
			Type:        types.StringValue(normalizeRuleType(item.Type)),
			CompareType: types.StringValue(normalizeCompareType(item.CompareType)),
			Value:       types.StringValue(item.Value),
			Status:      types.StringValue(strings.ToLower(item.Status)),
		}

		if item.RuleKey != "" {
			ruleItem.Key = types.StringValue(item.RuleKey)
		} else {
			ruleItem.Key = types.StringNull()
		}

		rules = append(rules, ruleItem)
	}

	if hasPolicyFilter {
		cfg.ID = types.StringValue(fmt.Sprintf("%d/%d/%d", vpcID, listenerID, filterPolicyID))
	} else {
		cfg.ID = types.StringValue(fmt.Sprintf("%d/%d", vpcID, listenerID))
	}
	cfg.Rules = rules
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
