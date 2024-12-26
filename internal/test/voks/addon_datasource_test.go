// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAddonDatasource(t *testing.T) {

	var (
		clusterId = 2555
		name      = "dashboard"
		status    = "inactive"
		version   = ""
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testAddonDataSourceConfig(clusterId, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon.testing", "cluster_id", strconv.Itoa(clusterId)),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon.testing", "name", name),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon.testing", "status", status),
					resource.TestCheckResourceAttr("data.viettelidc_voks_addon.testing", "version", version),
				),
			},
		},
	})
}

func testAddonDataSourceConfig(clusterId int, name string) string {
	return fmt.Sprintf(`
data "viettelidc_voks_addon" "testing" {
  cluster_id = %d
  name       = "%s"
}
`, clusterId, name)
}
