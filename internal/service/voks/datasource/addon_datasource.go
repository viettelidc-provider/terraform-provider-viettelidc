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
	_ datasource.DataSource              = &addonDatasource{}
	_ datasource.DataSourceWithConfigure = &addonDatasource{}
)

type addonDatasource struct {
	client *voks.APIClient
}

type AddonDataSourceModel struct {
	ClusterId types.Int32  `tfsdk:"cluster_id"`
	Name      types.String `tfsdk:"name"`
	Version   types.String `tfsdk:"version"`
	Status    types.String `tfsdk:"status"`
}

func NewAddonDataSource() datasource.DataSource {
	return &addonDatasource{}
}

func (a *addonDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

	a.client = voks.NewAPIClient(*cfg)
}

func (a *addonDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_addon"
}

func (a *addonDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retrieve information about a vOKS add-on within ViettelIdc.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.Int32Attribute{
				Description: "Id of the Cluster.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the Add-on.",
				Required:    true,
			},
			"version": schema.StringAttribute{
				Description: "Version of Add-on.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The current status of Add-on. Valid values: `ACTIVE`, `INACTIVE`, `INSTALLING`, `UNINSTALLING`.",
				Computed:    true,
			},
		},
	}
}

func (a *addonDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data AddonDataSourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	addonRes, _, err := a.client.AddOnApi.GetDetailAddon(ctx, data.ClusterId.ValueInt32(), data.Name.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Cluster Addon detail",
			"Could not read Cluster Addon detail, unexpected error: "+err.Error())
		return
	}

	data.Version = types.StringValue(addonRes.Version)
	data.Status = types.StringValue(addonRes.Status)

	diags = response.State.Set(ctx, &data)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
