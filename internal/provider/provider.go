// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/iam"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
	"os"
	voksDatasource "terraform-provider-viettelidc/internal/service/voks/datasource"
	voksResource "terraform-provider-viettelidc/internal/service/voks/resource"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &viettelidcProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &viettelidcProvider{
			version: version,
		}
	}
}

// viettelidcProvider is the provider implementation.
type viettelidcProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

type viettelidcProviderModel struct {
	Host     types.String `tfsdk:"host"`
	DomainId types.String `tfsdk:"domain_id"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	MfaCode  types.String `tfsdk:"mfa_code"`
}

// Metadata returns the provider type name.
func (p *viettelidcProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "viettelidc"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *viettelidcProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Viettel Kubernetes.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Optional: true,
			},
			"domain_id": schema.StringAttribute{
				Description: "DomainId for Viettel Kubernetes API.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username for Viettel Kubernetes API.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for Viettel Kubernetes API.",
				Optional:    true,
			},
			"mfa_code": schema.StringAttribute{
				Description: "Muti-factor Authentication code for Viettel Kubernetes API.",
				Optional:    true,
			},
		},
	}
}

// Configure prepares a Viettelidc API client for data sources and resources.
func (p *viettelidcProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve provider data from configuration
	var config viettelidcProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if config.DomainId.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Unknown Viettelidc API DomainId",
			"The provider cannot create the Viettelidc API client as there is an unknown configuration value for the Viettelidc API domain_id. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VIETTELIDC_DOMAIN_ID environment variable.",
		)
	}

	if config.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown Viettelidc API Username",
			"The provider cannot create the Viettelidc API client as there is an unknown configuration value for the Viettelidc API username. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VIETTELIDC_USERNAME environment variable.",
		)
	}

	if config.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown Viettelidc API Username",
			"The provider cannot create the Viettelidc API client as there is an unknown configuration value for the Viettelidc API password. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VIETTELIDC_PASSWORD environment variable.",
		)
	}

	if config.MfaCode.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Unknown Viettelidc API Username",
			"The provider cannot create the Viettelidc API client as there is an unknown configuration value for the Viettelidc API MFA Code. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VIETTELIDC_MFA_CODE environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	host := os.Getenv("VIETTELIDC_HOST")
	domainId := os.Getenv("VIETTELIDC_DOMAIN_ID")
	username := os.Getenv("VIETTELIDC_USERNAME")
	password := os.Getenv("VIETTELIDC_PASSWORD")
	mfaCode := os.Getenv("VIETTELIDC_MFA_CODE")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.DomainId.IsNull() {
		domainId = config.DomainId.ValueString()
	}

	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}

	if !config.MfaCode.IsNull() {
		mfaCode = config.MfaCode.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if host == "" {
		host = "https://cmp.viettelidc.com.vn"
	}

	if domainId == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("domain_id"),
			"Missing Viettelidc API DomainId",
			"The provider cannot create the Viettelidc API client as there is a missing or empty value for the Viettelidc API domainId. "+
				"Set the username value in the configuration or use the VIETTELIDC_DOMAIN_ID environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Missing Viettelidc API DomainId",
			"The provider cannot create the Viettelidc API client as there is a missing or empty value for the Viettelidc API username. "+
				"Set the username value in the configuration or use the VIETTELIDC_USERNAME environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Missing Viettelidc API Username",
			"The provider cannot create the Viettelidc API client as there is a missing or empty value for the Viettelidc API password. "+
				"Set the password value in the configuration or use the VIETTELIDC_PASSWORD environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	configuration := &viettelidc.Configuration{
		BasePath:      host,
		DefaultHeader: make(map[string]string),
		UserAgent:     "viettelidc/iac",
	}
	iamAPIClient := iam.NewAPIClient(configuration)

	loginRes, _, err := iamAPIClient.AuthorizationControllerApi.LoginViaLoginPage(ctx, iam.LoginViaLoginPageRequest{
		Username:     username,
		Password:     password,
		DomainId:     domainId,
		IsRememberMe: false,
		UserType:     "IAM_USER",
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Viettelidc API Client",
			"An unexpected error occurred when creating the Viettelidc API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Viettelidc Client Error: "+err.Error(),
		)
		return
	}

	if loginRes.IsRequiredSecondAuthenticationStep {

		if mfaCode == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("mfa_code"),
				"Missing Viettelidc API MfaCode",
				"The provider cannot create the Viettelidc API client as there is a missing or empty value for the Viettelidc API mfaCode. "+
					"Set the password value in the configuration or use the VIETTELIDC_MFA_CODE environment variable. "+
					"If either is already set, ensure the value is not empty.",
			)
			return
		}

		exchangeTokenRes, _, err := iamAPIClient.AuthorizationControllerApi.VerifyMfaTokenCode(ctx, iam.LoginViaPageWithMfaCodeRequest{
			MfaToken: loginRes.Data,
			MfaCode:  mfaCode,
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Create Viettelidc API Client",
				"An unexpected error occurred when creating the Viettelidc API client. "+
					"If the error is not clear, please contact the provider developers.\n\n"+
					"Viettelidc Client Error: "+err.Error(),
			)
			return
		}
		configuration.AccessToken = exchangeTokenRes.Data
	} else {
		configuration.AccessToken = loginRes.Data
	}

	accountRes, _, err := iamAPIClient.AccountClientApi.GetAccountInfoClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Viettelidc API Client",
			"An unexpected error occurred when creating the Viettelidc API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Viettelidc Client Error: "+err.Error(),
		)
		return
	}

	configuration.Id = accountRes.Data.Id
	configuration.DomainId = accountRes.Data.DomainId
	configuration.CustomerId = accountRes.Data.CustomerId

	//// Make the Viettelidc client available during DataSource and Resource
	//// type Configure methods.
	resp.DataSourceData = configuration
	resp.ResourceData = configuration
}

// DataSources defines the data sources implemented in the provider.
func (p *viettelidcProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		voksDatasource.NewKubeconfigResource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *viettelidcProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		//NewApiGatewayResource,
		voksResource.NewNodeGroupResource,
		voksResource.NewAddonResource,
		voksResource.NewClusterResource,
	}
}
