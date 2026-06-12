---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_route_table (Data Source)"
sidebar_current: "docs-viettelidc-datasource-route-table"
description: |-
  Retrieves information about an existing ViettelIDC Route Table.
---

# Data Source: viettelidc\_route\_table

Use this data source to look up a Route Table by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_route_table" "main" {
  name   = "main-rt"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Route Table ID. Either `id` or `name` must be set.
* `name` - (Optional) Route Table name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Route Table ID.
* `name` - Route Table name.
