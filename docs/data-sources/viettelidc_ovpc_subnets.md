---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_subnets (Data Source)"
sidebar_current: "docs-viettelidc-datasource-subnets"
description: |-
  Retrieves all Subnets in a ViettelIDC VPC.
---

# Data Source: viettelidc\_ovpc\_subnets

Use this data source to list all Subnets in a VPC.

## Example Usage

```hcl
data "viettelidc_ovpc_subnets" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}

output "subnet_ids" {
  value = [for s in data.viettelidc_ovpc_subnets.all.subnets : s.id]
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID filter. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `subnets` - List of subnets. Each item exports:
  * `id` - Subnet ID.
  * `name` - Subnet name.
  * `network_address` - CIDR.
  * `is_public_zone` - Whether the subnet is in the public zone.
  * `description` - Description.
