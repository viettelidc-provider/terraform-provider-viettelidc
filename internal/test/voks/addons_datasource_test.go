// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAddonsDatasource(t *testing.T) {

	var (
		name              = "dashboard"
		kubernetesVersion = "v1.29.8"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testAddonsDataSourceConfig(kubernetesVersion, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.viettelidc_voks_addons.testing", "kubernetes_version", kubernetesVersion),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addons.testing", "filter.name", name),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addons.testing", "names.#", "1"),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addons.testing", "names.0", name),
				),
			},
		},
	})
}

func testAddonsDataSourceConfig(kubernetesVersion, name string) string {
	return fmt.Sprintf(`
data "viettelidc_voks_addons" "testing" {
	kubernetes_version = "%s"
	filter = {
		name =  "%s"
	}
}
`, kubernetesVersion, name)
}
