// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package voks

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestClusterDatasource(t *testing.T) {

	var (
		id               = 2555
		endpoint         = "https://172.17.11.53:6443"
		name             = "k8s-idc-0de2cc72"
		cpu              = 2
		ipAddress        = "10.20.29.230"
		memory           = 2
		clusterStatus    = "POWERED_ON"
		totalStorageSize = 100
		status           = "SUCCESS"
		version          = "v1.29.8"
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + testClusterDataSourceConfig(id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "id", strconv.Itoa(id)),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "endpoint", endpoint),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "name", name),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "nfs.cpu", strconv.Itoa(cpu)),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "nfs.memory", strconv.Itoa(memory)),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "nfs.ip_address", ipAddress),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "nfs.total_storage_size", strconv.Itoa(totalStorageSize)),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "nfs.status", clusterStatus),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "status", status),
					resource.TestCheckResourceAttr("data.viettelidc_voks_cluster.testing", "version", version),
				),
			},
		},
	})
}

func testClusterDataSourceConfig(clusterId int) string {
	return fmt.Sprintf(`
data "viettelidc_voks_cluster" "testing" {
  id = %d
}
`, clusterId)
}
