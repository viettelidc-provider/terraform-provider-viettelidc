---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_instance (Data Source)"
sidebar_current: "docs-viettelidc-datasource-instance"
description: |-
  Retrieves information about an existing ViettelIDC Compute Instance.
---

# Data Source: viettelidc\_instance

Use this data source to look up a Compute Instance by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_instance" "vm" {
  id     = "vm-id"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Instance ID. Either `id` or `name` must be set.
* `name` - (Optional) Instance name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Instance ID.
* `name` - Instance name.
* `status` - Instance status.
* `ip_address` - Primary IP address.
* `root_nic_id` - Primary NIC ID.
* `image_id` - Image ID used.
* `image_name` - Image name.
* `cpu` - Number of vCPUs.
* `memory` - RAM in MB.
* `template_id` - Template ID.
* `availability_zone` - Availability zone.
