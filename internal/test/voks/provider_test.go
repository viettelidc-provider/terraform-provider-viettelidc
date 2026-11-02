// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"os"
	"terraform-provider-viettelidc/internal/provider"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var (
	// Credentials come from VIETTELIDC_DOMAIN_ID, VIETTELIDC_USERNAME and
	// VIETTELIDC_PASSWORD; see testAccPreCheck.
	providerConfig = `
provider "viettelidc" {}
`
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"viettelidc": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
)

func testAccPreCheck(t *testing.T) {
	for _, env := range []string{"VIETTELIDC_DOMAIN_ID", "VIETTELIDC_USERNAME", "VIETTELIDC_PASSWORD"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests", env)
		}
	}
}
