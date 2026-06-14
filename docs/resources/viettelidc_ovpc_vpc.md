---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_vpc"
sidebar_current: "docs-viettelidc-resource-vpc"
description: |-
  Provides a ViettelIDC Virtual Private Cloud (VPC).
---

# viettelidc\_ovpc\_vpc

Provides a Virtual Private Cloud (VPC) resource on ViettelIDC. A VPC is an isolated virtual network that hosts subnets, instances, and other networking resources.

## Example Usage

```hcl
resource "viettelidc_ovpc_vpc" "main" {
  name        = "my-vpc"
  cidr_block  = "10.0.0.0/16"
  description = "Main project VPC"
}
```

## Argument Reference

* `name` - (Required) Name of the VPC.
* `cidr_block` - (Required, ForceNew) IP address range in CIDR notation (e.g. `10.0.0.0/16`). **Cannot be changed after creation.**
* `description` - (Optional) Description of the VPC.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - VPC ID assigned by the platform.
* `status` - VPC status (`success`, `pending`, `error`, ...).

## Import

VPCs can be imported using the VPC ID:

```
terraform import viettelidc_ovpc_vpc.main <vpc_id>
```
