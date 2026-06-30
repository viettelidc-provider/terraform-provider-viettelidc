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

type clusterNetworkDataSource struct {
	clientData *providerdata.ProviderData
}

type ClusterNetworkDSModel struct {
	ID                  types.String   `tfsdk:"id"`
	ClusterID           types.String   `tfsdk:"cluster_id"`
	MasterSecurityGroup types.String   `tfsdk:"master_security_group"`
	WorkerSecurityGroup types.String   `tfsdk:"worker_security_group"`
	VIPLoadBalancer     types.String   `tfsdk:"vip_load_balancer"`
	IngressList         []types.String `tfsdk:"ingress_list"`
}

func NewClusterNetworkDataSource() datasource.DataSource {
	return &clusterNetworkDataSource{}
}

func (d *clusterNetworkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vks_cluster_network"
}

func (d *clusterNetworkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *clusterNetworkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for VKS Cluster Network details.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
			},
			"master_security_group": schema.StringAttribute{
				Computed: true,
			},
			"worker_security_group": schema.StringAttribute{
				Computed: true,
			},
			"vip_load_balancer": schema.StringAttribute{
				Computed: true,
			},
			"ingress_list": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *clusterNetworkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ClusterNetworkDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"clusterId":  config.ClusterID.ValueString(),
		"customerId": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathClusterDetailNetwork, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = config.ClusterID
		config.MasterSecurityGroup = types.StringValue(asString(dataMap, "masterSecurityGroup"))
		config.WorkerSecurityGroup = types.StringValue(asString(dataMap, "workerSecurityGroup"))
		config.VIPLoadBalancer = types.StringValue(asString(dataMap, "vipLoadBalancer"))

		config.IngressList = make([]types.String, 0)
		if ingresses, ok := dataMap["ingressList"].([]interface{}); ok {
			for _, ing := range ingresses {
				if s, ok := ing.(string); ok {
					config.IngressList = append(config.IngressList, types.StringValue(s))
				}
			}
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	} else {
		resp.Diagnostics.AddError("Parse Error", "Failed to parse cluster network details response")
	}
}
