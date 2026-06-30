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

var (
	_ datasource.DataSource              = &nodeGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &nodeGroupDataSource{}
)

func NewNodeGroupDataSource() datasource.DataSource {
	return &nodeGroupDataSource{}
}

type nodeGroupDataSource struct {
	clientData *providerdata.ProviderData
}

func (d *nodeGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_node_group"
}

func (d *nodeGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *nodeGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Node Group lookup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the Node Group.",
				Required:    true,
			},
			"cluster_id": schema.StringAttribute{
				Description: "ID of the K8s Cluster.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the Node Group.",
				Computed:    true,
			},
			"instance_type": schema.StringAttribute{
				Description: "Instance type of the workers.",
				Computed:    true,
			},
			"min_size": schema.Int64Attribute{
				Description: "Minimum size of the Node Group.",
				Computed:    true,
			},
			"max_size": schema.Int64Attribute{
				Description: "Maximum size of the Node Group.",
				Computed:    true,
			},
			"desired_size": schema.Int64Attribute{
				Description: "Desired size of the Node Group.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the Node Group.",
				Computed:    true,
			},
		},
	}
}

func (d *nodeGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state NodeGroupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"cluster_id":  state.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathNodeGroupDetail, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		state.Name = types.StringValue(asString(dataMap, "name"))
		state.Status = types.StringValue(asString(dataMap, "status"))
		if cId := asString(dataMap, "clusterId"); cId != "" {
			state.ClusterID = types.StringValue(cId)
		} else if cId := asString(dataMap, "vttClusterId"); cId != "" {
			state.ClusterID = types.StringValue(cId)
		}
		instType := asString(dataMap, "instanceType")
		if instType == "" {
			instType = asString(dataMap, "code")
		}
		if instType != "" {
			state.InstanceType = types.StringValue(instType)
		}

		minSize := asInt64(dataMap, "minSize")
		if minSize == 0 {
			minSize = asInt64(dataMap, "minNode")
		}
		state.MinSize = types.Int64Value(minSize)

		maxSize := asInt64(dataMap, "maxSize")
		if maxSize == 0 {
			maxSize = asInt64(dataMap, "maxNode")
		}
		state.MaxSize = types.Int64Value(maxSize)

		desiredSize := asInt64(dataMap, "desiredSize")
		if desiredSize == 0 {
			desiredSize = asInt64(dataMap, "replicas")
		}
		state.DesiredSize = types.Int64Value(desiredSize)

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Could not parse node group detail response data")
	}
}
