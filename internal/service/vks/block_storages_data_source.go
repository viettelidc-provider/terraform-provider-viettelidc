// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-viettelidc/internal/service/vopc/providerdata"
)

type clusterBlockStoragesDataSource struct {
	clientData *providerdata.ProviderData
}

type BlockStorageModel struct {
	ID     types.String `json:"id" tfsdk:"id"`
	Name   types.String `json:"name" tfsdk:"name"`
	Size   types.Int64  `json:"size" tfsdk:"size"`
	Status types.String `json:"status" tfsdk:"status"`
}

type ClusterBlockStoragesDSModel struct {
	ID            types.String        `tfsdk:"id"`
	ClusterID     types.String        `tfsdk:"cluster_id"`
	BlockStorages []BlockStorageModel `tfsdk:"block_storages"`
}

func NewClusterBlockStoragesDataSource() datasource.DataSource {
	return &clusterBlockStoragesDataSource{}
}

func (d *clusterBlockStoragesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster_block_storages"
}

func (d *clusterBlockStoragesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterBlockStoragesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing Block Storages attached to a VKS Cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"block_storages": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"size": schema.Int64Attribute{
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

func (d *clusterBlockStoragesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterBlockStoragesDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"pageIndex":  0,
		"pageSize":   1000,
		"filters":    []interface{}{},
		"clusterId":  config.ClusterID.ValueString(),
		"hostId":     6,
		"customerId": d.clientData.CustomerID,
		"planType":   "k8s",
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathK8sBlockStorageList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listEnvelope struct {
		Items []map[string]interface{} `json:"items"`
	}

	if err := json.Unmarshal(apiResp.Data, &listEnvelope); err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Failed to decode block storages response: %v", err))
		return
	}

	config.ID = config.ClusterID
	config.BlockStorages = make([]BlockStorageModel, 0, len(listEnvelope.Items))

	for _, item := range listEnvelope.Items {
		idVal := fmt.Sprintf("%v", item["id"])
		if idVal == "<nil>" || idVal == "" {
			continue
		}

		bs := BlockStorageModel{
			ID:     types.StringValue(idVal),
			Name:   types.StringValue(asString(item, "name")),
			Size:   types.Int64Value(asInt64(item, "size")),
			Status: types.StringValue(asString(item, "status")),
		}
		config.BlockStorages = append(config.BlockStorages, bs)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
