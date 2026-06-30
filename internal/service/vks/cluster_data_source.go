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
	_ datasource.DataSource              = &clusterDataSource{}
	_ datasource.DataSourceWithConfigure = &clusterDataSource{}
)

func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}

type clusterDataSource struct {
	clientData *providerdata.ProviderData
}

func (d *clusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster"
}

func (d *clusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Cluster lookup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the Cluster.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the Cluster.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Status of the Cluster.",
				Computed:    true,
			},
			"version": schema.StringAttribute{
				Description: "Kubernetes version.",
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "API endpoint.",
				Computed:    true,
			},
			"vpc_id": schema.StringAttribute{
				Description: "VPC ID of the Cluster.",
				Computed:    true,
			},
		},
	}
}

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state ClusterResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"id":          state.ID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterDetail, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		state.Name = types.StringValue(asString(dataMap, "clusterName"))
		if state.Name.IsNull() || state.Name.ValueString() == "" {
			state.Name = types.StringValue(asString(dataMap, "name"))
		}
		state.Status = types.StringValue(asString(dataMap, "status"))
		state.Version = types.StringValue(asString(dataMap, "version"))
		state.Endpoint = types.StringValue(asString(dataMap, "apiAddress"))
		state.VpcID = types.StringValue(asString(dataMap, "vpcId"))

		if vpcConfig, ok := dataMap["vpcConfig"].(map[string]interface{}); ok {
			state.VpcID = types.StringValue(asString(vpcConfig, "vpcId"))
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Could not parse cluster detail response data")
	}
}
