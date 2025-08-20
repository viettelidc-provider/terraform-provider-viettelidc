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
	_ datasource.DataSource              = &vpcDatasource{}
	_ datasource.DataSourceWithConfigure = &vpcDatasource{}
)

type VpcDatasourceModel struct {
	VpcId  types.Int32  `tfsdk:"vpc_id"`
	ID     types.Int32  `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
	Tier   types.String `tfsdk:"tierid"`
}

type vpcDatasource struct {
	client *vpc.APIClient
}

func NewVpcDatasource() datasource.DataSource {
	return &vpcDatasource{}
}

func (a *vpcDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (a *vpcDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_vpc"
}

func (a *vpcDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retrieve list of available VCPs within ViettelIdc.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.Int32Attribute{
				Description: "VPC ID to filter the results.",
				Required:    true,
			},
			"id": schema.Int32Attribute{
				Description: "Id of the VPC.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the VPC.",
				Computed:    true,
			},
			"tierid": schema.StringAttribute{
				Description: "VPC tier.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of VPC. Valid values: `Success`, `Suspended`.",
				Computed:    true,
			},
		},
	}
}

func (a *vpcDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data VpcDatasourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	reqBody := vpc.VpcGetItemRequest{
		Id:    data.ID.ValueInt32(),
		VpcId: data.VpcId.ValueInt32(),
	}
	result, httpResp, err := a.client.VirtualPrivateCloudApi.VpcGetDetail(ctx, reqBody)
	if err != nil {
		response.Diagnostics.AddError("Error calling API", err.Error())
		return
	}
	if httpResp != nil && httpResp.Body != nil {
		defer httpResp.Body.Close()
	}

	var state VpcDatasourceModel
	// Copy the input parameters and set the response data
	state.VpcId = data.VpcId
	state.ID = data.ID  // Keep the input ID
	state.Name = types.StringValue(result.Name)
	state.Status = types.StringValue(result.Status)
	state.Tier = types.StringValue(fmt.Sprint(result.TierId)) // convert int32 → string

	diags = response.State.Set(ctx, state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
