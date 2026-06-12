---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_route_table"
sidebar_current: "docs-viettelidc-resource-route-table"
description: |-
  Provides a ViettelIDC Route Table.
---

# viettelidc\_route\_table

Provides a Route Table on ViettelIDC. Use `viettelidc_route_table_association` to associate subnets with the route table.

~> **Note:** The API has no delete endpoint for route tables. Destroying this resource removes it from Terraform state only — the route table is **not deleted** on the cloud.

## Example Usage

```hcl
resource "viettelidc_route_table" "main" {
  name   = "main-rt"
  vpc_id = viettelidc_vpc.main.id
}

resource "viettelidc_route_table_association" "assoc" {
  route_table_id = viettelidc_route_table.main.id
  subnet_id      = viettelidc_subnet.private.id
  vpc_id         = viettelidc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Route Table name.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Route Table ID (`vttRouteTableId`).

## Import

Route Tables can be imported using the Route Table ID:

```
terraform import viettelidc_route_table.main <route_table_id>
```
