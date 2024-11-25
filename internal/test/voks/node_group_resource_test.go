// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestNodeGroupResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + testNodeGroupResourceConfig(false, false, 1, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "name", "iac-unit-test"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "resource_type", "T1.vOKS 1"),

					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "auto_repair", "false"),

					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.enable_auto_scale", "false"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.min_node", "1"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.max_node", "1"),

					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "labels.environment", "production"),

					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "taint.0.key", "dedicated"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "taint.0.value", "gpu"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "taint.0.effect", "NoSchedule"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "viettelidc_voks_node_group.testing",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					rs, ok := state.RootModule().Resources["viettelidc_voks_node_group.testing"]
					if !ok {
						return "", fmt.Errorf("resource not found")
					}
					return fmt.Sprintf("%s,%s", rs.Primary.Attributes["id"], rs.Primary.Attributes["cluster_id"]), nil
				},
			},
			// Update and Read testing
			{
				Config: providerConfig + testNodeGroupResourceConfig(true, true, 1, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "auto_repair", "true"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.enable_auto_scale", "true"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.min_node", "1"),
					resource.TestCheckResourceAttr("viettelidc_voks_node_group.testing", "scaling_config.max_node", "3"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testNodeGroupResourceConfig(enableAutoScale, autoRepair bool, minNode, maxNode int) string {
	return fmt.Sprintf(`
resource "viettelidc_voks_node_group" "testing" {

    cluster_id = 2477
    name = "iac-unit-test"
    resource_type = "T1.vOKS 1"

    scaling_config {
        enable_auto_scale = %t
        min_node = %d
        max_node = %d
    }

    auto_repair = %t

    labels = {
        environment = "production"
    }

    taint {
        key    = "dedicated"
        value  = "gpu"
        effect = "NoSchedule"
    }
}
`, enableAutoScale, minNode, maxNode, autoRepair)
}
