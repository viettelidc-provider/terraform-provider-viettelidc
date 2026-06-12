---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_vpc (Data Source)"
sidebar_current: "docs-viettelidc-datasource-vpc"
description: |-
  Retrieves information about an existing ViettelIDC VPC.
---

# Data Source: viettelidc\_vpc

Use this data source to retrieve information about an existing Virtual Private Cloud (VPC) by `id` or `name`. Useful when the VPC is provisioned outside Terraform.

## Example Usage

```hcl
data "viettelidc_vpc" "main" {
  name = "my-vpc"
}

output "vpc_cidr" {
  value = data.viettelidc_vpc.main.cidr_block
}
```

## Argument Reference

* `id` - (Optional) VPC ID. Either `id` or `name` must be set.
* `name` - (Optional) VPC name. Either `id` or `name` must be set.

## Attributes Reference

* `id` - VPC ID.
* `name` - VPC name.
* `cidr_block` - CIDR block of the VPC.
* `description` - VPC description.
* `status` - VPC status.
