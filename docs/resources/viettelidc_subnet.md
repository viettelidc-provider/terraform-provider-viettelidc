---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_subnet"
sidebar_current: "docs-viettelidc-resource-subnet"
description: |-
  Provides a ViettelIDC Subnet inside a VPC.
---

# viettelidc\_subnet

Provides a Subnet resource inside a VPC on ViettelIDC.

## Example Usage

```hcl
resource "viettelidc_vpc" "main" {
  name       = "my-vpc"
  cidr_block = "10.0.0.0/16"
}

resource "viettelidc_subnet" "private" {
  name            = "private-subnet"
  network_address = "10.0.1.0/24"
  is_public_zone  = false
  vpc_id          = viettelidc_vpc.main.id
  description     = "Private subnet"
}

resource "viettelidc_subnet" "public" {
  name            = "public-subnet"
  network_address = "10.0.2.0/24"
  is_public_zone  = true
  vpc_id          = viettelidc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Human-readable subnet name.
* `network_address` - (Required, ForceNew) CIDR network address (e.g. `10.0.1.0/24`). **Cannot be changed after creation.**
* `is_public_zone` - (Optional, ForceNew) Whether the subnet is in the public zone. Defaults to `false`. **Cannot be changed after creation.**
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.
* `description` - (Optional) Description of the subnet.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Subnet ID (`vttSubnetId`).

## Import

Subnets can be imported using the subnet ID:

```
terraform import viettelidc_subnet.private <subnet_id>
```
