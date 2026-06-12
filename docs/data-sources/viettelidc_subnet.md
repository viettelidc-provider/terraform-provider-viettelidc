---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_subnet (Data Source)"
sidebar_current: "docs-viettelidc-datasource-subnet"
description: |-
  Retrieves information about an existing ViettelIDC Subnet.
---

# Data Source: viettelidc\_subnet

Use this data source to look up a single Subnet by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_subnet" "private" {
  name   = "private-subnet"
  vpc_id = data.viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Subnet ID. Either `id` or `name` must be set.
* `name` - (Optional) Subnet name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID to search within. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Subnet ID.
* `name` - Subnet name.
* `network_address` - CIDR of the subnet.
* `is_public_zone` - Whether the subnet is in the public zone.
* `description` - Description.
