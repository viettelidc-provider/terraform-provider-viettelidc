---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_network_interface (Data Source)"
sidebar_current: "docs-viettelidc-datasource-network-interface"
description: |-
  Retrieves information about an existing ViettelIDC Network Interface (NIC).
---

# Data Source: viettelidc\_network\_interface

Use this data source to look up a Network Interface by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_network_interface" "nic" {
  id     = "nic-id"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) NIC ID. Either `id` or `name` must be set.
* `name` - (Optional) NIC name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - NIC ID.
* `name` - NIC name.
* `subnet_id` - ID of the attached subnet.
* `ip_assign_type` - IP assignment type (`STATIC` or `auto`).
* `ip_address` - IP address.
* `description` - Description.
* `status` - NIC status.
