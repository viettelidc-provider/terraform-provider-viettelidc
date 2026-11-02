// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestKubeconfigDatasource(t *testing.T) {

	var (
		clusterId = 2459
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testKubeconfigDataSourceConfig(clusterId),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.viettelidc_voks_kubeconfig.testing", "cluster_id", strconv.Itoa(clusterId)),
					resource.TestCheckResourceAttrSet("data.viettelidc_voks_kubeconfig.testing", "value"),
				),
			},
		},
	})
}

func testKubeconfigDataSourceConfig(clusterId int) string {
	return fmt.Sprintf(`
data "viettelidc_voks_kubeconfig" "testing" {
    cluster_id = %d
}
`, clusterId)
}
