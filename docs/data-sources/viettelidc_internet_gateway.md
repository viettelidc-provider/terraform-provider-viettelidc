---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_internet_gateway (Data Source)"
sidebar_current: "docs-viettelidc-datasource-internet-gateway"
description: |-
  Retrieves information about an existing ViettelIDC Internet Gateway.
---

# Data Source: viettelidc\_internet\_gateway

Use this data source to look up an Internet Gateway by `id` or `name`. Internet Gateways are platform-managed resources — they can only be listed and referenced, not created or deleted via Terraform.

## Example Usage

```hcl
data "viettelidc_internet_gateway" "igw" {
  name   = "default-igw"
  vpc_id = viettelidc_vpc.main.id
}

resource "viettelidc_nat_gateway" "nat" {
  name                = "main-nat"
  subnet_id           = viettelidc_subnet.public.id
  internet_gateway_id = data.viettelidc_internet_gateway.igw.id
  vpc_id              = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Internet Gateway ID. Either `id` or `name` must be set.
* `name` - (Optional) Internet Gateway name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Internet Gateway ID.
* `name` - Name.
* `status` - Current status.
* `subnet_id` - Associated subnet ID.
* `floating_ip` - Floating IP address.
