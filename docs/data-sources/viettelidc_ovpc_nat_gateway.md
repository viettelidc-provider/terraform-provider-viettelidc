---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_nat_gateway (Data Source)"
sidebar_current: "docs-viettelidc-datasource-nat-gateway"
description: |-
  Retrieves information about an existing ViettelIDC NAT Gateway.
---

# Data Source: viettelidc\_ovpc\_nat\_gateway

Use this data source to look up a NAT Gateway by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_ovpc_nat_gateway" "nat" {
  name   = "main-nat"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) NAT Gateway ID. Either `id` or `name` must be set.
* `name` - (Optional) NAT Gateway name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - NAT Gateway ID.
* `name` - Name.
* `subnet_id` - Subnet ID.
* `internet_gateway_id` - Internet Gateway ID.
* `connect_type` - Connection type.
* `floating_ip` - Floating IP address.
* `floating_ip_id` - Floating IP ID.
* `status` - Status.
* `created_at` - Creation timestamp.
