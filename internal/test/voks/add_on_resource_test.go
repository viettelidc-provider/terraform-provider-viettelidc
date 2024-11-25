// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAddonResource(t *testing.T) {

	var (
		clusterId = 2477
		name      = "grafana"
		version   = "8.0.2"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testAddonResourceConfig(clusterId, name, version),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("viettelidc_voks_addon.grafana", "cluster_id", strconv.Itoa(clusterId)),
					resource.TestCheckResourceAttr("viettelidc_voks_addon.grafana", "name", name),
					resource.TestCheckResourceAttr("viettelidc_voks_addon.grafana", "version", version),
					resource.TestCheckResourceAttr("viettelidc_voks_addon.grafana", "status", "active"),
				),
			},
			// ImportState testing
			{
				Config:                               providerConfig + testAddonResourceConfig(clusterId, name, version),
				ResourceName:                         "viettelidc_voks_addon.grafana",
				ImportState:                          true,
				ImportStateVerify:                    false,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        strconv.Itoa(clusterId) + "," + name,
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAddonResourceConfig(clusterId int, name, version string) string {
	return fmt.Sprintf(`
resource "viettelidc_voks_addon" "grafana" {
    cluster_id = %d
    name = "%s"
    version= "%s"
}
`, clusterId, name, version)
}
