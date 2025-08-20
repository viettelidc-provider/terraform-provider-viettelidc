// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/vpc"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
)

var (
	_ datasource.DataSource              = &vpcQuotaLimitDatasource{}
	_ datasource.DataSourceWithConfigure = &vpcQuotaLimitDatasource{}
)

type vpcQuotaLimitDatasource struct {
	client *vpc.APIClient
}

type VpcQuotaLimitDatasourceModel struct {
	VpcId types.Int32  `tfsdk:"vpc_id"`
	Items []QuotaModel `tfsdk:"items"`
}
type QuotaModel struct {
	Name  types.String `tfsdk:"name"`
	Unit  types.String `tfsdk:"unit"`
	Value types.Int32  `tfsdk:"value"`
	Usage types.Int32  `tfsdk:"usage"`
}

func NewVpcQuotaLimitDatasource() datasource.DataSource {
	return &vpcQuotaLimitDatasource{}
}

func (a *vpcQuotaLimitDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

	a.client = vpc.NewAPIClient(cfg)
}

func (a *vpcQuotaLimitDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_vpc_quota_limits"
}

func (a *vpcQuotaLimitDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retrieve list of VPC quota limits for a specific VPC within ViettelIdc.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.Int32Attribute{
				Description: "VPC ID to filter the results.",
				Required:    true,
			},
			"items": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of VPC quotas.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name of the VPC quota.",
							Computed:    true,
						},
						"unit": schema.StringAttribute{
							Description: "Unit of the VPC quota.",
							Computed:    true,
						},
						"value": schema.Int32Attribute{
							Description: "VPC quota maximum value.",
							Computed:    true,
						},
						"usage": schema.Int32Attribute{
							Description: "The current usage of the VPC quota.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (a *vpcQuotaLimitDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data VpcQuotaLimitDatasourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	reqBody := vpc.VpcGetQuotaRequest{
		VpcId: data.VpcId.ValueInt32(),
	}

	result, httpResp, err := a.client.VirtualPrivateCloudApi.VpcGetQuotaLimit(ctx, reqBody)
	if err != nil {
		response.Diagnostics.AddError("Error calling API", fmt.Sprintf("API Error: %v", err))
		return
	}
	if httpResp != nil && httpResp.Body != nil {
		defer httpResp.Body.Close()
	}

	var state VpcQuotaLimitDatasourceModel
	state.VpcId = data.VpcId

	// Process the API response
	for _, item := range result.Items {
		quotaItem := QuotaModel{
			Name:  types.StringValue(item.Name),
			Unit:  types.StringValue(item.Unit),
			Value: types.Int32Value(int32(item.Value)),
			Usage: types.Int32Value(int32(item.Usage)),
		}
		state.Items = append(state.Items, quotaItem)
	}

	diags = response.State.Set(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
