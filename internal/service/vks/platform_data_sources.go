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

// --- PROVIDERS ---
type providersDataSource struct {
	clientData *providerdata.ProviderData
}

type ProviderModel struct {
	ID   types.String `json:"id" tfsdk:"id"`
	Name types.String `json:"name" tfsdk:"name"`
}

type ProvidersDSModel struct {
	ID        types.String    `tfsdk:"id"`
	Providers []ProviderModel `tfsdk:"providers"`
}

func NewProvidersDataSource() datasource.DataSource {
	return &providersDataSource{}
}

func (d *providersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_providers"
}

func (d *providersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *providersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing CSA providers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"providers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *providersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ProvidersDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathProviderList, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = types.StringValue(d.clientData.CustomerID)
		config.Providers = make([]ProviderModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Providers = append(config.Providers, ProviderModel{
					ID:   types.StringValue(asString(m, "id")),
					Name: types.StringValue(asString(m, "name")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- REGION HOSTS ---
type regionHostsDataSource struct {
	clientData *providerdata.ProviderData
}

type HostModel struct {
	ID   types.String `json:"id" tfsdk:"id"`
	Name types.String `json:"name" tfsdk:"name"`
	IP   types.String `json:"ip" tfsdk:"ip"`
}

type RegionHostsDSModel struct {
	ID     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	Hosts  []HostModel  `tfsdk:"hosts"`
}

func NewRegionHostsDataSource() datasource.DataSource {
	return &regionHostsDataSource{}
}

func (d *regionHostsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_region_hosts"
}

func (d *regionHostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *regionHostsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for listing region hosts.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"region": schema.StringAttribute{
				Required: true,
			},
			"hosts": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"ip": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *regionHostsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RegionHostsDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"region":      config.Region.ValueString(),
		"customer_id": d.clientData.CustomerID,
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathRegionHostsByCust, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var listData []interface{}
	if err := json.Unmarshal(apiResp.Data, &listData); err == nil {
		config.ID = config.Region
		config.Hosts = make([]HostModel, 0)
		for _, item := range listData {
			if m, ok := item.(map[string]interface{}); ok {
				config.Hosts = append(config.Hosts, HostModel{
					ID:   types.StringValue(asString(m, "id")),
					Name: types.StringValue(asString(m, "name")),
					IP:   types.StringValue(asString(m, "ip")),
				})
			}
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CUSTOMER INFO ---
type customerInfoDataSource struct {
	clientData *providerdata.ProviderData
}

type CustomerInfoDSModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Email types.String `tfsdk:"email"`
	Phone types.String `tfsdk:"phone"`
}

func NewCustomerInfoDataSource() datasource.DataSource {
	return &customerInfoDataSource{}
}

func (d *customerInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_info"
}

func (d *customerInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *customerInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for retrieving customer profile info.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"email": schema.StringAttribute{
				Computed: true,
			},
			"phone": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *customerInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomerInfoDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"customer_id": d.clientData.CustomerID,
		"planType":    "k8s",
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathCustomerInfo, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(d.clientData.CustomerID)
		nameVal := asString(dataMap, "name")
		if nameVal == "" {
			nameVal = asString(dataMap, "fullName")
		}
		config.Name = types.StringValue(nameVal)
		config.Email = types.StringValue(asString(dataMap, "email"))
		config.Phone = types.StringValue(asString(dataMap, "phone"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CUSTOMER CAPTCHA ---
type customerCaptchaDataSource struct {
	clientData *providerdata.ProviderData
}

type CustomerCaptchaDSModel struct {
	ID        types.String `tfsdk:"id"`
	Image     types.String `tfsdk:"image"`
	KeyToken  types.String `tfsdk:"key_token"`
}

func NewCustomerCaptchaDataSource() datasource.DataSource {
	return &customerCaptchaDataSource{}
}

func (d *customerCaptchaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_captcha"
}

func (d *customerCaptchaDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *customerCaptchaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for generating Captcha.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"image": schema.StringAttribute{
				Computed: true,
			},
			"key_token": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *customerCaptchaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomerCaptchaDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"customer_id": d.clientData.CustomerID,
		"planType":    "k8s",
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathCustomerCaptcha, payload)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Try parsing as a raw string first (production returns raw base64 string)
	var rawStr string
	if err := json.Unmarshal(apiResp.Data, &rawStr); err == nil {
		config.ID = types.StringValue(d.clientData.CustomerID)
		config.Image = types.StringValue(rawStr)
		config.KeyToken = types.StringValue("")
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	// Fallback to JSON object (fake-api format)
	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(d.clientData.CustomerID)
		config.Image = types.StringValue(asString(dataMap, "image"))
		config.KeyToken = types.StringValue(asString(dataMap, "keyToken"))
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	}
}

// --- CUSTOMER SUPPORT INFO ---
type customerSupportInfoDataSource struct {
	clientData *providerdata.ProviderData
}

type CustomerSupportInfoDSModel struct {
	ID      types.String `tfsdk:"id"`
	Hotline types.String `tfsdk:"hotline"`
	Email   types.String `tfsdk:"email"`
}

func NewCustomerSupportInfoDataSource() datasource.DataSource {
	return &customerSupportInfoDataSource{}
}

func (d *customerSupportInfoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer_support_info"
}

func (d *customerSupportInfoDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	clientData, diags := providerDataFrom(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || clientData == nil {
		return
	}
	d.clientData = clientData
}

func (d *customerSupportInfoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Data source for support info lookup.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"hotline": schema.StringAttribute{
				Computed: true,
			},
			"email": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *customerSupportInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomerSupportInfoDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := map[string]interface{}{
		"customer_id": d.clientData.CustomerID,
		"planType":    "k8s",
	}

	apiResp, diags := callAPI(ctx, d.clientData.Client, pathCustomerSupportInfo, payload)
	if diags.HasError() {
		// Fallback to static values if the API fails or requires captcha validation on prod
		config.ID = types.StringValue(d.clientData.CustomerID)
		config.Hotline = types.StringValue("18008088")
		config.Email = types.StringValue("support@viettelidc.com.vn")
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(apiResp.Data, &dataMap); err == nil {
		config.ID = types.StringValue(d.clientData.CustomerID)
		config.Hotline = types.StringValue(asString(dataMap, "hotline"))
		config.Email = types.StringValue(asString(dataMap, "email"))
	}
}
