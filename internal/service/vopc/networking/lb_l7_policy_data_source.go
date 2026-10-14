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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-viettelidc/internal/service/vopc/client"
)

var (
	_ datasource.DataSource              = (*L7PolicyDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*L7PolicyDataSource)(nil)
)

type L7PolicyDataSource struct {
	client       *client.Client
	customerID   string
	defaultVpcID string
}

func NewL7PolicyDataSource() datasource.DataSource {
	return &L7PolicyDataSource{}
}

type L7PolicyDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	VpcID          types.Int64  `tfsdk:"vpc_id"`
	ListenerID     types.Int64  `tfsdk:"listener_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Action         types.String `tfsdk:"action"`
	RedirectURL    types.String `tfsdk:"redirect_url"`
	RedirectPoolID types.Int64  `tfsdk:"redirect_pool_id"`
	Priority       types.Int64  `tfsdk:"priority"`
	Status         types.String `tfsdk:"status"`
	NumberOfRules  types.Int64  `tfsdk:"number_of_rules"`
}

func (d *L7PolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ovpc_lb_l7_policy"
}

func (d *L7PolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a single L7 policy by ID or name on a listener.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.Int64Attribute{
				Description: "VPC ID.",
				Required:    true,
			},
			"listener_id": schema.Int64Attribute{
				Description: "Listener ID.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "Policy ID to look up.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Policy name to look up.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Policy description.",
				Computed:     true,
			},
			"action": schema.StringAttribute{
				Description: "Policy action.",
				Computed:     true,
			},
			"redirect_url": schema.StringAttribute{
				Description: "Redirect URL for REDIRECT_TO_URL action.",
				Computed:     true,
			},
			"redirect_pool_id": schema.Int64Attribute{
				Description: "Pool ID for REDIRECT_TO_POOL action.",
				Computed:     true,
			},
			"priority": schema.Int64Attribute{
				Description: "Policy priority.",
				Computed:     true,
			},
			"status": schema.StringAttribute{
				Description: "Policy status.",
				Computed:     true,
			},
			"number_of_rules": schema.Int64Attribute{
				Description: "Number of rules attached to this policy.",
				Computed:     true,
			},
		},
	}
}

func (d *L7PolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	pd, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if pd == nil {
		return
	}
	d.client = pd.Client
	d.customerID = pd.CustomerID
	d.defaultVpcID = pd.DefaultVpcID
}

func (d *L7PolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg L7PolicyDataSourceModel
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

func (d *L7PolicyDataSource) lookup(ctx context.Context, cfg *L7PolicyDataSourceModel) (*L7PolicyDataSourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	hasID := !cfg.ID.IsNull() && !cfg.ID.IsUnknown() && cfg.ID.ValueString() != ""
	hasName := !cfg.Name.IsNull() && !cfg.Name.IsUnknown() && cfg.Name.ValueString() != ""

	if (hasID && hasName) || (!hasID && !hasName) {
		diags.AddError("Invalid filter", "Exactly one of 'id' or 'name' must be specified.")
		return nil, diags
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

	apiResp, dDiags := callAPI(ctx, d.client, pathL7PolicyList, body)
	diags.Append(dDiags...)
	if diags.HasError() {
		return nil, diags
	}

	var listEnvelope struct {
		PageIndex  int                      `json:"pageIndex"`
		PageSize   int                      `json:"pageSize"`
		TotalItems int                      `json:"totalItems"`
		Items      []l7PolicyDetailResponse `json:"items"`
	}

	if apiResp != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &listEnvelope); err != nil {
			diags.AddError("Parse Policy List Failed", err.Error())
			return nil, diags
		}
	}

	var found *l7PolicyDetailResponse
	if hasName {
		wantedName := cfg.Name.ValueString()
		for i := range listEnvelope.Items {
			if listEnvelope.Items[i].Name == wantedName {
				found = &listEnvelope.Items[i]
				break
			}
		}
	} else {
		wantedIDStr := cfg.ID.ValueString()
		wantedID, _ := strconv.ParseInt(wantedIDStr, 10, 64)
		for i := range listEnvelope.Items {
			pID := listEnvelope.Items[i].VttLoadBalancerL7PolicyId
			if pID == 0 && listEnvelope.Items[i].ID != nil {
				pID = parseID(listEnvelope.Items[i].ID)
			}
			if pID == wantedID || fmt.Sprintf("%v", listEnvelope.Items[i].ID) == wantedIDStr {
				found = &listEnvelope.Items[i]
				break
			}
		}
	}

	if found == nil && hasID {
		policyID, err := strconv.ParseInt(cfg.ID.ValueString(), 10, 64)
		if err == nil && policyID > 0 {
			detailBody := map[string]interface{}{
				"vpcId":                     vpcID,
				"vpc_id":                    vpcID,
				"customerId":                d.customerID,
				"customer_id":               d.customerID,
				"vttLoadBalancerListenerId": listenerID,
				"vttLoadBalancerL7PolicyId": policyID,
				"id":                        policyID,
			}
			detailResp, d2 := callAPI(ctx, d.client, pathL7PolicyDetail, detailBody)
			if !d2.HasError() && detailResp != nil && len(detailResp.Data) > 0 && string(detailResp.Data) != "null" {
				var detail l7PolicyDetailResponse
				if err := json.Unmarshal(detailResp.Data, &detail); err == nil {
					found = &detail
				}
			}
		}
	}

	if found == nil {
		if hasName {
			diags.AddError("Policy Not Found", fmt.Sprintf("L7 policy with name=%q not found on listener %d", cfg.Name.ValueString(), listenerID))
		} else {
			diags.AddError("Policy Not Found", fmt.Sprintf("L7 policy with id=%q not found on listener %d", cfg.ID.ValueString(), listenerID))
		}
		return nil, diags
	}

	pID := found.VttLoadBalancerL7PolicyId
	if pID == 0 && found.ID != nil {
		pID = parseID(found.ID)
	}

	res := &L7PolicyDataSourceModel{
		ID:            types.StringValue(strconv.FormatInt(pID, 10)),
		VpcID:         types.Int64Value(vpcID),
		ListenerID:    types.Int64Value(listenerID),
		Name:          types.StringValue(found.Name),
		Action:        types.StringValue(normalizeAction(found.Action)),
		Status:        types.StringValue(strings.ToLower(found.Status)),
		NumberOfRules: types.Int64Value(found.NumberOfRules),
	}
	if found.Description != "" {
		res.Description = types.StringValue(found.Description)
	} else {
		res.Description = types.StringNull()
	}
	if found.RedirectURL != "" {
		res.RedirectURL = types.StringValue(found.RedirectURL)
	} else {
		res.RedirectURL = types.StringNull()
	}
	if found.VttLoadBalancerPoolId > 0 {
		res.RedirectPoolID = types.Int64Value(found.VttLoadBalancerPoolId)
	} else {
		res.RedirectPoolID = types.Int64Null()
	}
	if found.Priority != nil {
		res.Priority = types.Int64Value(*found.Priority)
	} else {
		res.Priority = types.Int64Null()
	}

	return res, diags
}
