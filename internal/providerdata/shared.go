// Package providerdata defines the shared data type published to all resources
// and data sources via Configure(). Placing it in its own package prevents
// import cycles between internal/provider and the service packages.
package providerdata

import (
	iac_providerdata "terraform-provider-viettelidc/internal/service/iac/providerdata"

	"github.com/viettelidc-provider/viettelidc-api-client-go/viettelidc"
)

// SharedProviderData is the value stored in resp.ResourceData /
// resp.DataSourceData by the provider's Configure() method.
//
// VOKS resources assert VoksConfig.
// IaC (viettelidc_*) resources assert IacData.
type SharedProviderData struct {
	// VoksConfig is the API client configuration used by voks_* resources.
	VoksConfig *viettelidc.Configuration

	// IacData carries the custom HTTP client used by viettelidc_* resources.
	IacData *iac_providerdata.ProviderData
}
