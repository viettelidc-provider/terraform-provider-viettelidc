---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_route_table_association"
sidebar_current: "docs-viettelidc-resource-route-table-association"
description: |-
  Associates a Subnet with a ViettelIDC Route Table.
---

# viettelidc\_ovpc\_route\_table\_association

Associates a Subnet with a Route Table on ViettelIDC.

## Example Usage

```hcl
resource "viettelidc_ovpc_route_table_association" "assoc" {
  route_table_id = viettelidc_ovpc_route_table.main.id
  subnet_id      = viettelidc_ovpc_subnet.private.id
  vpc_id         = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `route_table_id` - (Required, ForceNew) ID of the Route Table.
* `subnet_id` - (Required, ForceNew) ID of the Subnet to associate.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Composite ID in the format `<route_table_id>/<subnet_id>`.

## Import

Route Table Associations can be imported using the composite ID:

```
terraform import viettelidc_ovpc_route_table_association.assoc <route_table_id>/<subnet_id>
```
