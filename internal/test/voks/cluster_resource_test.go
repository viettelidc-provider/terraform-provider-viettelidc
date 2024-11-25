// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterResource(t *testing.T) {

	var (
		clusterId = 2459
		name      = "k8s-idc-162e0ab7"
		version   = "v1.30.5"
		vpc_id    = 19178
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// ImportState testing
			{
				Config: providerConfig + testClusterResourceConfig(name, version, vpc_id, 0),
				Check: resource.ComposeAggregateTestCheckFunc(
					//Check resource attribute value with terraform state
					resource.TestCheckResourceAttr("viettelidc_voks_cluster.testing", "nfs.total_storage_size", strconv.Itoa(100)),
				),
			},
			{
				Config:                               providerConfig + testClusterResourceConfig(name, version, vpc_id, 0),
				ResourceName:                         "viettelidc_voks_cluster.testing",
				ImportState:                          true,
				ImportStateVerifyIdentifierAttribute: "id",
				ImportStateId:                        strconv.Itoa(clusterId),
			},
			// Update and Read testing
			{
				Config: providerConfig + testClusterResourceConfig(name, version, vpc_id, 50),
				Check: resource.ComposeAggregateTestCheckFunc(
					//Check resource attribute value with terraform state
					resource.TestCheckResourceAttr("viettelidc_voks_cluster.testing", "nfs.total_storage_size", strconv.Itoa(100)),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testClusterResourceConfig(name, version string, vpcId, nfsAdditionalSize int) string {

	var nfsConfig string
	if nfsAdditionalSize > 0 {
		nfsConfig = fmt.Sprintf(`
nfs = {
	additional_storage_size = %d
}`, nfsAdditionalSize)
	}

	return fmt.Sprintf(`
resource "viettelidc_voks_cluster" "testing" {
	name = "%s"
	version = "%s"
	vpc_config {
		vpc_id = %d
	}
	%s
}`, name, version, vpcId, nfsConfig)
}
