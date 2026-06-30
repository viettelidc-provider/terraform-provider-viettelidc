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

// --- NODEGROUP LABELS ---
type nodegroupLabelsDataSource struct {
	clientData *providerdata.ProviderData
}

type NodegroupLabelsDSModel struct {
	ID        types.String      `tfsdk:"id"`
	ClusterID types.String      `tfsdk:"cluster_id"`
	GroupID   types.String      `tfsdk:"group_id"`
	Labels    map[string]string `tfsdk:"labels"`
}

func NewNodegroupLabelsDataSource() datasource.DataSource {
	return &nodegroupLabelsDataSource{}
}

func (d *nodegroupLabelsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nodegroup_labels"
}

func (d *nodegroupLabelsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *nodegroupLabelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Node Group Labels.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"group_id": schema.StringAttribute{
				Required: true,
			},
			"labels": schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *nodegroupLabelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodegroupLabelsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"groupId":    config.GroupID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathNodegroupLabelsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.GroupID.ValueString())
		config.Labels = make(map[string]string)
		for k, v := range dataMap {
			if s, ok := v.(string); ok {
				config.Labels[k] = s
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- NODEGROUP TAINTS ---
type nodegroupTaintsDataSource struct {
	clientData *providerdata.ProviderData
}

type TaintModel struct {
	Key    types.String `json:"key" tfsdk:"key"`
	Value  types.String `json:"value" tfsdk:"value"`
	Effect types.String `json:"effect" tfsdk:"effect"`
}

type NodegroupTaintsDSModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	GroupID   types.String `tfsdk:"group_id"`
	Taints    []TaintModel `tfsdk:"taints"`
}

func NewNodegroupTaintsDataSource() datasource.DataSource {
	return &nodegroupTaintsDataSource{}
}

func (d *nodegroupTaintsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nodegroup_taints"
}

func (d *nodegroupTaintsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *nodegroupTaintsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Node Group Taints.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"group_id": schema.StringAttribute{
				Required: true,
			},
			"taints": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Computed: true,
						},
						"value": schema.StringAttribute{
							Computed: true,
						},
						"effect": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *nodegroupTaintsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodegroupTaintsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"groupId":    config.GroupID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathNodegroupTaintsList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.GroupID.ValueString())
		config.Taints = make([]TaintModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Taints = append(config.Taints, TaintModel{
					Key:    types.StringValue(asString(m, "key")),
					Value:  types.StringValue(asString(m, "value")),
					Effect: types.StringValue(asString(m, "effect")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- NODEGROUP TEMPLATES ---
type nodegroupTemplatesDataSource struct {
	clientData *providerdata.ProviderData
}

type TemplateModel struct {
	ID   types.String `json:"id" tfsdk:"id"`
	Name types.String `json:"name" tfsdk:"name"`
}

type NodegroupTemplatesDSModel struct {
	ID        types.String    `tfsdk:"id"`
	ClusterID types.String    `tfsdk:"cluster_id"`
	Templates []TemplateModel `tfsdk:"templates"`
}

func NewNodegroupTemplatesDataSource() datasource.DataSource {
	return &nodegroupTemplatesDataSource{}
}

func (d *nodegroupTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nodegroup_templates"
}

func (d *nodegroupTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *nodegroupTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Node Group Templates.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"templates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *nodegroupTemplatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NodegroupTemplatesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathNodegroupTemplatesList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = config.ClusterID
		config.Templates = make([]TemplateModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Templates = append(config.Templates, TemplateModel{
					ID:   types.StringValue(asString(m, "id")),
					Name: types.StringValue(asString(m, "name")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CLUSTER NODE DETAIL ---
type clusterNodeDetailDataSource struct {
	clientData *providerdata.ProviderData
}

type ClusterNodeDetailDSModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	NodeID    types.String `tfsdk:"node_id"`
	Name      types.String `tfsdk:"name"`
	IPAddress types.String `tfsdk:"ip_address"`
	Status    types.String `tfsdk:"status"`
}

func NewClusterNodeDetailDataSource() datasource.DataSource {
	return &clusterNodeDetailDataSource{}
}

func (d *clusterNodeDetailDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster_node_detail"
}

func (d *clusterNodeDetailDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterNodeDetailDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving details of a single VKS Cluster Node.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"node_id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"ip_address": schema.StringAttribute{
				Computed: true,
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *clusterNodeDetailDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterNodeDetailDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"nodeId":     config.NodeID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterNodeDetail, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(config.ClusterID.ValueString() + "/" + config.NodeID.ValueString())
		config.Name = types.StringValue(asString(dataMap, "name"))
		config.IPAddress = types.StringValue(asString(dataMap, "ipAddress"))
		config.Status = types.StringValue(asString(dataMap, "status"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CLUSTER NODES ---
type clusterNodesDataSource struct {
	clientData *providerdata.ProviderData
}

type NodeModel struct {
	ID        types.String `json:"id" tfsdk:"id"`
	Name      types.String `json:"name" tfsdk:"name"`
	IPAddress types.String `json:"ip_address" tfsdk:"ip_address"`
	Status    types.String `json:"status" tfsdk:"status"`
}

type ClusterNodesDSModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Nodes     []NodeModel  `tfsdk:"nodes"`
}

func NewClusterNodesDataSource() datasource.DataSource {
	return &clusterNodesDataSource{}
}

func (d *clusterNodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_nodes"
}

func (d *clusterNodesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterNodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing VKS Cluster Nodes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"nodes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ip_address": schema.StringAttribute{
							Computed: true,
						},
						"status": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *clusterNodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterNodesDSModel
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
		"sorts":       []interface{}{},
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterNodesList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiRespMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &apiRespMap); err == nil {
		config.ID = config.ClusterID
		config.Nodes = make([]NodeModel, 0)
		if items, ok := apiRespMap["items"].([]interface{}); ok {
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					config.Nodes = append(config.Nodes, NodeModel{
						ID:        types.StringValue(asString(m, "id")),
						Name:      types.StringValue(asString(m, "name")),
						IPAddress: types.StringValue(asString(m, "ipAddress")),
						Status:    types.StringValue(asString(m, "status")),
					})
				}
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}
