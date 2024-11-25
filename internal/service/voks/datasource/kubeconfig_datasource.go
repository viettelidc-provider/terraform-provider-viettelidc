// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasource

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/voks"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
)

var (
	_ datasource.DataSource              = &kubeconfigDatasource{}
	_ datasource.DataSourceWithConfigure = &kubeconfigDatasource{}
)

type kubeconfigDatasource struct {
	client *voks.APIClient
}

type KubeconfigDataSourceModel struct {
	ClusterId types.Int32  `tfsdk:"cluster_id"`
	Value     types.String `tfsdk:"value"`
}

func NewKubeconfigResource() datasource.DataSource {
	return &kubeconfigDatasource{}
}

func (k *kubeconfigDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	// Add a nil check when handling ProviderData because Terraform
	// sets that data after it calls the ConfigureProvider RPC.
	if request.ProviderData == nil {
		return
	}

	cfg, ok := request.ProviderData.(*viettelidc.Configuration)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *apiclient.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)

		return
	}

	k.client = voks.NewAPIClient(*cfg)
}

func (k *kubeconfigDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_kubeconfig"
}

func (k *kubeconfigDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "The configuration for accessing the cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.Int32Attribute{
				Description: "Id of the ViettelIdc Kuberneters Cluster.",
				Required: true,
			},
			"value": schema.StringAttribute{
				Description: "The kubeconfig file is essential for configuring access to the cluster, providing connection details, authentication credentials, and other configurations.",
				Computed: true,
			},
		},
	}
}

func (k *kubeconfigDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {

	var data KubeconfigDataSourceModel
	// Read Terraform configuration data into the model
	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)

	res, _, err := k.client.ClusterApi.KubeConfigCluster(ctx, voks.BaseResourceReq{
		ClusterId: data.ClusterId.ValueInt32(),
	})
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to Read Kubeconfig Info",
			err.Error(),
		)
		return
	}

	data.Value = types.StringValue(res.KubeConfig)

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}
