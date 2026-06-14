---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_nat_gateway"
sidebar_current: "docs-viettelidc-resource-nat-gateway"
description: |-
  Provides a ViettelIDC NAT Gateway.
---

# viettelidc\_ovpc\_nat\_gateway

Provides a NAT Gateway on ViettelIDC. A NAT Gateway allows instances in a private subnet to connect to the Internet through an Internet Gateway.

## Example Usage

```hcl
data "viettelidc_ovpc_internet_gateway" "igw" {
  name   = "default-igw"
  vpc_id = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_nat_gateway" "nat" {
  name                = "main-nat"
  subnet_id           = viettelidc_ovpc_subnet.public.id
  internet_gateway_id = data.viettelidc_ovpc_internet_gateway.igw.id
  connect_type        = false
  vpc_id              = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Human-readable NAT Gateway name.
* `subnet_id` - (Required, ForceNew) Subnet ID where the NAT Gateway will be placed.
* `internet_gateway_id` - (Required, ForceNew) ID of the Internet Gateway to use for outbound traffic.
* `connect_type` - (Optional, ForceNew) Connection type. `true` uses a dedicated connection. **Cannot be changed after creation.**
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - NAT Gateway ID (`vttNatId`).
* `floating_ip` - Floating IP address assigned to the NAT Gateway.
* `floating_ip_id` - ID of the Floating IP.
* `status` - NAT Gateway status (`ACTIVE`, `PENDING`, ...).
* `created_at` - Timestamp when the NAT Gateway was created.

## Import

NAT Gateways can be imported using the NAT Gateway ID:

```
terraform import viettelidc_ovpc_nat_gateway.nat <nat_gateway_id>
```
