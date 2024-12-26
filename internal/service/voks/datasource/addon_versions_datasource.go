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
	_ datasource.DataSource              = &addonVersionsDatasource{}
	_ datasource.DataSourceWithConfigure = &addonVersionsDatasource{}
)

type addonVersionsDatasource struct {
	client *voks.APIClient
}

type AddonVersionsDataSourceModel struct {
	Name              types.String         `tfsdk:"name"`
	KubernetesVersion types.String         `tfsdk:"kubernetes_version"`
	Filter            *AddOnVersionsFilter `tfsdk:"filter"`
	Versions          types.List           `tfsdk:"versions"`
}

type AddOnVersionsFilter struct {
	Version types.String `tfsdk:"version"`
}

func NewAddonVersionsDataSource() datasource.DataSource {
	return &addonVersionsDatasource{}
}

func (a *addonVersionsDatasource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
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

func (a *addonVersionsDatasource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_voks_addon_versions"
}

func (a *addonVersionsDatasource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "The Addon Versions data source allows you to get information about addon versions.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Name of the Addon.",
				Required:    true,
			},
			"kubernetes_version": schema.StringAttribute{
				Description: "Version of Kubernetes. Must be between 1-100 characters in length. Must begin with an alphanumeric character, and must only contain alphanumeric characters, dashes and underscore (^[0-9A-Za-z][A-Za-z0-9\\-_]+$)",
				Required:    true,
			},
			"filter": schema.SingleNestedAttribute{
				Description: "Filter the Addon versions by their version name.",
				Required:    false,
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"version": schema.StringAttribute{
						Description: "Version of the Addon.",
						Required:    true,
					},
				},
			},
			"versions": schema.ListAttribute{
				Description: "List of Addon versions.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (a *addonVersionsDatasource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {

	var data AddonVersionsDataSourceModel
	diags := request.Config.Get(ctx, &data)
	response.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	opts := &voks.AddOnApiGetAllAddonVersionOpts{}
	if data.Filter != nil {
		opts.Version = optional.NewString(data.Filter.Version.ValueString())
	}

	addonVersions, _, err := a.client.AddOnApi.GetAllAddonVersion(ctx, data.Name.ValueString(), data.KubernetesVersion.ValueString(), opts)
	if err != nil {
		response.Diagnostics.AddError("Error reading Addon Versions",
			"Could not read Addon Versions, unexpected error: "+err.Error())
		return
	}

	var versions []types.String
	for _, addonVersion := range addonVersions {
		versions = append(versions, types.StringValue(addonVersion.VersionName))
	}

	data.Versions, diags = types.ListValueFrom(ctx, types.StringType, versions)
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
