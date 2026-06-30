// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package providerdata holds the shared struct passed from the CSA provider's
// Configure() into every resource/data source. It lives in its own package to
// avoid an import cycle between the csa package (which registers resources)
// and the networking package (which consumes the data).
package providerdata

import "terraform-provider-viettelidc/internal/service/vopc/client"

// ProviderData is the value injected into each resource/data source via
// resp.ResourceData / resp.DataSourceData. Resources type-assert it back to
// *ProviderData inside their Configure() method.
type ProviderData struct {
	// Client is the configured CSA HTTP client (already carrying base URL
	// and auth token).
	Client *client.Client

	// CustomerID is the tenant identifier injected into every CSA request body.
	CustomerID string

	// DefaultVpcID is an optional default applied when a resource block does
	// NOT specify its own vpc_id. Empty string means "no default"; resources
	// must then fail-fast at plan/apply time.
	DefaultVpcID string

	// HostID is the host identifier configured in the provider.
	HostID int64
}
