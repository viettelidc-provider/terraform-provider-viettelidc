// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package datasource

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/voks"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
)

var (
	_ datasource.DataSource              = &addonsDatasource{}
	_ datasource.DataSourceWithConfigure = &addonsDatasource{}
)

type addonsDatasource struct {
	client *voks.APIClient
}

type AddonsDataSourceModel struct {
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
	Names             types.List   `tfsdk:"names"`
	Filter            *Filter      `tfsdk:"filter"`
}

type Filter struct {
	Name types.String `tfsdk:"name"`
}

func NewAddonsDataSource() datasource.DataSource {
	return &addonsDatasource{}
}

func (a *addonsDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (a *addonsDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_addons"
}

func (a *addonsDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Retieve information about a vOKS Add-on within ViettelIdc",
		Attributes: map[string]schema.Attribute{
			"kubernetes_version": schema.StringAttribute{
				Description: "Version of Kubernetes.",
				Required:    true,
			},
			"filter": schema.SingleNestedAttribute{
				Description: "Filter the Addon by its name.",
				Required:    false,
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Description: "Name of the Addon.",
						Required:    true,
					},
				},
			},
			"names": schema.ListAttribute{
				Description: "Name of the Addon.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (a *addonsDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {

	var data AddonsDataSourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	opts := &voks.AddOnApiGetAllAddOnOpts{}
	if data.Filter != nil {
		opts.Name = optional.NewString(data.Filter.Name.ValueString())
	}

	var names []types.String
	addons, _, err := a.client.AddOnApi.GetAllAddOn(ctx, data.KubernetesVersion.ValueString(), opts)

	if err != nil {
		response.Diagnostics.AddError(
			"Error reading Addons",
			"Could not read Addons, unexpected error: "+err.Error())
		return
	}

	for _, addon := range addons {
		names = append(names, types.StringValue(addon.AddOnName))
	}

	data.Names, diags = types.ListValueFrom(ctx, types.StringType, names)
	if diags.HasError() {
		response.Diagnostics.Append(diags...)
		return
	}

	diags = response.State.Set(ctx, &data)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}
}
