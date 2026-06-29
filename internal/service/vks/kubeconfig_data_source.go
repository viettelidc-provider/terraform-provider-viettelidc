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

type kubeconfigDataSource struct {
	clientData *providerdata.ProviderData
}

type KubeconfigDSModel struct {
	ID         types.String `tfsdk:"id"`
	ClusterID  types.String `tfsdk:"cluster_id"`
	Kubeconfig types.String `tfsdk:"kubeconfig"`
}

func NewKubeconfigDataSource() datasource.DataSource {
	return &kubeconfigDataSource{}
}

func (d *kubeconfigDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_kubeconfig"
}

func (d *kubeconfigDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *kubeconfigDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving VKS Cluster Kubeconfig.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"kubeconfig": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func (d *kubeconfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config KubeconfigDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"cluster_id":  config.ClusterID.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	// For standard download-config API, POST `/csa/api/v1/kubernetes/cluster/download-config`
	apiResp, diags := callAPI(ctx, d.clientData.Client, pathKubeconfigDownload, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = config.ClusterID
		config.Kubeconfig = types.StringValue(asString(dataMap, "kubeConfig"))
		if config.Kubeconfig.IsNull() || config.Kubeconfig.ValueString() == "" {
			config.Kubeconfig = types.StringValue(asString(dataMap, "kubeconfig"))
		}
		if config.Kubeconfig.IsNull() || config.Kubeconfig.ValueString() == "" {
			// Fallback: use raw data if it is not encapsulated in JSON field
			config.Kubeconfig = types.StringValue(string(apiResp.Data))
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	} else {
		config.ID = config.ClusterID
		config.Kubeconfig = types.StringValue(string(apiResp.Data))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}
