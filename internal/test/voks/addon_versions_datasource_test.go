// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAddonVersionsDatasource(t *testing.T) {

	var (
		kubernetes_version = "v1.29.8"
		name               = "dashboard"
		version            = "6.0.8"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testAddonVersionDataSourceConfig(name, kubernetes_version),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon_versions.testing", "name", name),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon_versions.testing", "kubernetes_version", kubernetes_version),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon_versions.testing", "versions.0", version),
				),
			},
		},
	})
}

func testAddonVersionDataSourceConfig(name, version string) string {
	return fmt.Sprintf(`
data "viettelidc_voks_addon_versions" "testing" {
    name = "%s"
    kubernetes_version = "%s"
}
`, name, version)
}
