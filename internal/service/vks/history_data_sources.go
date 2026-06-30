// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

// --- AUTOSCALE HISTORY ---
type autoscaleHistoryDataSource struct {
	clientData *providerdata.ProviderData
}

type AutoscaleHistoryModel struct {
	Timestamp types.String `json:"timestamp" tfsdk:"timestamp"`
	Action    types.String `json:"action" tfsdk:"action"`
	Message   types.String `json:"message" tfsdk:"message"`
}

type AutoscaleHistoryDSModel struct {
	ID        types.String            `tfsdk:"id"`
	ClusterID types.String            `tfsdk:"cluster_id"`
	History   []AutoscaleHistoryModel `tfsdk:"history"`
}

func NewAutoscaleHistoryDataSource() datasource.DataSource {
	return &autoscaleHistoryDataSource{}
}

func (d *autoscaleHistoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_autoscale_history"
}

func (d *autoscaleHistoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *autoscaleHistoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Cluster Autoscale history.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"history": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timestamp": schema.StringAttribute{
							Computed: true,
						},
						"action": schema.StringAttribute{
							Computed: true,
						},
						"message": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *autoscaleHistoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AutoscaleHistoryDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id": config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
		"pageIndex": 0,
		"pageSize": 100,
		"filters": []interface{}{},
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathAutoscaleHistory, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.History = make([]AutoscaleHistoryModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					config.History = append(config.History, AutoscaleHistoryModel{
						Timestamp: types.StringValue(asString(m, "timestamp")),
						Action:    types.StringValue(asString(m, "action")),
						Message:   types.StringValue(asString(m, "message")),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CLUSTER EVENTS ---
type clusterEventsDataSource struct {
	clientData *providerdata.ProviderData
}

type ClusterEventModel struct {
	ID        types.String `json:"id" tfsdk:"id"`
	Type      types.String `json:"type" tfsdk:"type"`
	Message   types.String `json:"message" tfsdk:"message"`
	Timestamp types.String `json:"timestamp" tfsdk:"timestamp"`
}

type ClusterEventsDSModel struct {
	ID        types.String        `tfsdk:"id"`
	ClusterID types.String        `tfsdk:"cluster_id"`
	Events    []ClusterEventModel `tfsdk:"events"`
}

func NewClusterEventsDataSource() datasource.DataSource {
	return &clusterEventsDataSource{}
}

func (d *clusterEventsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster_events"
}

func (d *clusterEventsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterEventsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Cluster Events.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"events": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"type": schema.StringAttribute{
							Computed: true,
						},
						"message": schema.StringAttribute{
							Computed: true,
						},
						"timestamp": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *clusterEventsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterEventsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
		"pageIndex":   0,
		"pageSize":    100,
		"filters":     []interface{}{},
		"startDate":   "01/01/2026",
		"endDate":     "31/12/2026",
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterEventsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.Events = make([]ClusterEventModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					config.Events = append(config.Events, ClusterEventModel{
						ID:        types.StringValue(asString(m, "id")),
						Type:      types.StringValue(asString(m, "type")),
						Message:   types.StringValue(asString(m, "message")),
						Timestamp: types.StringValue(asString(m, "timestamp")),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}
