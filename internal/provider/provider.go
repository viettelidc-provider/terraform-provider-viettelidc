// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/viettelidc-provider/viettelidc-api-client-go/service/iam"
	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"

	iac_client "terraform-provider-viettelidc/internal/service/iac/client"
	iac_providerdata "terraform-provider-viettelidc/internal/service/iac/providerdata"
	iacNetworking "terraform-provider-viettelidc/internal/service/iac/networking"
	iacVpc "terraform-provider-viettelidc/internal/service/iac/vpc"
	sharedpd "terraform-provider-viettelidc/internal/providerdata"
	voksDatasource "terraform-provider-viettelidc/internal/service/voks/datasource"
	voksResource "terraform-provider-viettelidc/internal/service/voks/resource"
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
	DomainId types.String `tfsdk:"domain_id"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	MfaCode  types.String `tfsdk:"mfa_code"`
	BaseURL  types.String `tfsdk:"base_url"`
	VpcID    types.String `tfsdk:"vpc_id"`
}

// Metadata returns the provider type name.
func (p *viettelidcProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "viettelidc"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *viettelidcProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with ViettelIdc resource.",
		Attributes: map[string]schema.Attribute{
			"domain_id": schema.StringAttribute{
				Description: "DomainId for ViettelIdc API.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username for ViettelIdc API.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for ViettelIdc API.",
				Optional:    true,
			},
			"mfa_code": schema.StringAttribute{
				Description: "Muti-factor Authentication code for ViettelIdc API.",
				Optional:    true,
			},
			"base_url": schema.StringAttribute{
				Description: "IaC API Gateway base URL. Default: https://iac.viettelidc.com.vn. Env: VIETTELIDC_BASE_URL.",
				Optional:    true,
			},
			"vpc_id": schema.StringAttribute{
				Description: "Default VPC ID for IaC resources. Env: VIETTELIDC_VPC_ID.",
				Optional:    true,
			},
		},
	}
}

// Configure prepares a Viettelidc API client for data sources and resources.
//
// Auth strategy:
//   - IaC resources (viettelidc_ovpc_*): always use username+password via
//     IaC client's own LoginWithPassword flow (no domain_id required).
//   - VOKS resources (voks_*): use IAM SDK login (domain_id required).
//     If domain_id is not provided, VoksConfig will be nil and voks_*
//     resources will return an error when accessed.
func (p *viettelidcProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config viettelidcProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ── Resolve config values with env-var fallbacks ─────────────────────────
	username := os.Getenv("VIETTELIDC_USERNAME")
	password := os.Getenv("VIETTELIDC_PASSWORD")
	domainId := os.Getenv("VIETTELIDC_DOMAIN_ID")
	mfaCode := os.Getenv("VIETTELIDC_MFA_CODE")

	if !config.Username.IsNull() && !config.Username.IsUnknown() {
		username = config.Username.ValueString()
	}
	if !config.Password.IsNull() && !config.Password.IsUnknown() {
		password = config.Password.ValueString()
	}
	if !config.DomainId.IsNull() && !config.DomainId.IsUnknown() {
		domainId = config.DomainId.ValueString()
	}
	if !config.MfaCode.IsNull() && !config.MfaCode.IsUnknown() {
		mfaCode = config.MfaCode.ValueString()
	}

	iacBaseURL := os.Getenv("VIETTELIDC_BASE_URL")
	if iacBaseURL == "" {
		iacBaseURL = "https://iac.viettelidc.com.vn"
	}
	if !config.BaseURL.IsNull() && !config.BaseURL.IsUnknown() {
		iacBaseURL = config.BaseURL.ValueString()
	}

	iacVpcID := os.Getenv("VIETTELIDC_VPC_ID")
	if !config.VpcID.IsNull() && !config.VpcID.IsUnknown() {
		iacVpcID = config.VpcID.ValueString()
	}

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Missing username",
			"Set username in provider config or VIETTELIDC_USERNAME env var.",
		)
	}
	if password == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Missing password",
			"Set password in provider config or VIETTELIDC_PASSWORD env var.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// ── IaC auth: LoginWithPassword (no domain_id needed) ────────────────────
	oldToken, accessToken, err := iac_client.LoginWithPassword(ctx, &http.Client{}, iacBaseURL, iac_client.LoginCredentials{
		Username: username,
		Password: password,
		UserType: "ROOT_USER",
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"IaC Login Failed",
			"Could not authenticate with IaC API: "+err.Error(),
		)
		return
	}

	iacHTTPClient := iac_client.NewClientWithTokens(iacBaseURL, oldToken, accessToken)
	iacData := &iac_providerdata.ProviderData{
		Client:       iacHTTPClient,
		DefaultVpcID: iacVpcID,
	}

	// Auto-extract customer_id from the JWT when not set explicitly.
	customerID := os.Getenv("VIETTELIDC_CUSTOMER_ID")
	if extracted, err := iac_client.ExtractCustomerIDFromJWT(oldToken); err == nil && extracted != "" {
		customerID = extracted
	}
	iacData.CustomerID = customerID

	// ── VOKS auth: IAM SDK (only when domain_id is present) ──────────────────
	var voksConfig *viettelidc.Configuration
	if domainId != "" {
		voksHost := os.Getenv("VIETTELIDC_HOST")
		if voksHost == "" {
			voksHost = "https://api.viettelidc.com.vn"
		}
		configuration := &viettelidc.Configuration{
			BasePath:      voksHost,
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
			resp.Diagnostics.AddError("VOKS Login Failed", err.Error())
			return
		}

		if loginRes.IsRequiredSecondAuthenticationStep {
			if mfaCode == "" {
				resp.Diagnostics.AddAttributeError(path.Root("mfa_code"), "MFA Required", "Set mfa_code or VIETTELIDC_MFA_CODE.")
				return
			}
			exchangeRes, _, err := iamAPIClient.AuthorizationControllerApi.VerifyMfaTokenCode(ctx, iam.LoginViaPageWithMfaCodeRequest{
				MfaToken: loginRes.Data,
				MfaCode:  mfaCode,
			})
			if err != nil {
				resp.Diagnostics.AddError("VOKS MFA Failed", err.Error())
				return
			}
			configuration.AccessToken = exchangeRes.Data
		} else {
			configuration.AccessToken = loginRes.Data
		}

		accountRes, _, err := iamAPIClient.AccountClientApi.GetAccountInfoClient(ctx)
		if err != nil {
			resp.Diagnostics.AddError("VOKS GetAccount Failed", err.Error())
			return
		}
		configuration.Id = accountRes.Data.Id
		configuration.DomainId = accountRes.Data.DomainId
		configuration.CustomerId = accountRes.Data.CustomerId
		iacData.CustomerID = configuration.CustomerId
		voksConfig = configuration
	}

	shared := &sharedpd.SharedProviderData{
		VoksConfig: voksConfig,
		IacData:    iacData,
	}
	resp.DataSourceData = shared
	resp.ResourceData = shared
}

// DataSources defines the data sources implemented in the provider.
func (p *viettelidcProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// VOKS
		voksDatasource.NewClusterDataSource,
		voksDatasource.NewKubeconfigResource,
		voksDatasource.NewNodeGroupDatasource,
		voksDatasource.NewAddonDataSource,
		voksDatasource.NewAddonsDataSource,
		voksDatasource.NewAddonVersionsDataSource,
		// IaC networking
		iacNetworking.NewSubnetDataSource,
		iacNetworking.NewSubnetsDataSource,
		iacNetworking.NewVPCDataSource,
		iacNetworking.NewFloatingIPDataSource,
		iacNetworking.NewLoadBalancerDataSource,
		iacNetworking.NewNatGatewayDataSource,
		iacNetworking.NewSecurityGroupDataSource,
		iacNetworking.NewNetworkInterfaceDataSource,
		iacNetworking.NewNetworkInterfacesDataSource,
		iacNetworking.NewRouteTableDataSource,
		iacNetworking.NewInternetGatewayDataSource,
		iacNetworking.NewInstanceDataSource,
		iacNetworking.NewVMTemplatesDataSource,
		iacNetworking.NewVFirewallsDataSource,
		iacNetworking.NewCertificateDataSource,
		iacNetworking.NewBackupRecordDataSource,
		iacNetworking.NewSGRuleTypesDataSource,
		// IaC vpc
		iacVpc.NewLaunchTemplateDataSource,
		iacVpc.NewLaunchTemplatesDataSource,
		iacVpc.NewAutoscaleGroupsDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *viettelidcProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// VOKS
		voksResource.NewClusterResource,
		voksResource.NewNodeGroupResource,
		voksResource.NewAddonResource,
		// IaC networking
		iacNetworking.NewSubnetResource,
		iacNetworking.NewVPCResource,
		iacNetworking.NewFloatingIPResource,
		iacNetworking.NewLoadBalancerResource,
		iacNetworking.NewNatGatewayResource,
		iacNetworking.NewSecurityGroupResource,
		iacNetworking.NewSecurityGroupRuleResource,
		iacNetworking.NewNetworkInterfaceResource,
		iacNetworking.NewNetworkInterfaceAttachmentResource,
		iacNetworking.NewRouteTableResource,
		iacNetworking.NewRouteTableAssociationResource,
		iacNetworking.NewKeyPairResource,
		iacNetworking.NewInstanceResource,
		iacNetworking.NewVolumeResource,
		iacNetworking.NewVolumeAttachmentResource,
		iacNetworking.NewCertificateResource,
		iacNetworking.NewBackupPlanResource,
		// IaC vpc
		iacVpc.NewAutoscaleGroupResource,
		iacVpc.NewLaunchTemplateResource,
	}
}
