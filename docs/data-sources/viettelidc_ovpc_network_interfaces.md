---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_network_interfaces (Data Source)"
sidebar_current: "docs-viettelidc-datasource-network-interfaces"
description: |-
  Retrieves all Network Interfaces in a ViettelIDC VPC.
---

# Data Source: viettelidc\_ovpc\_network\_interfaces

Use this data source to list all Network Interfaces (NICs) in a VPC.

## Example Usage

```hcl
data "viettelidc_ovpc_network_interfaces" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID filter. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `network_interfaces` - List of NICs. Each item exports:
  * `id` - NIC ID.
  * `name` - NIC name.
  * `subnet_id` - Subnet ID.
  * `ip_address` - IP address.
  * `status` - Status.
