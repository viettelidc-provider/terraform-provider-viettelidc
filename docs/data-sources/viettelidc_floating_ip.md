---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_floating_ip (Data Source)"
sidebar_current: "docs-viettelidc-datasource-floating-ip"
description: |-
  Retrieves information about an existing ViettelIDC Floating IP.
---

# Data Source: viettelidc\_floating\_ip

Use this data source to look up a Floating IP by its ID (`vttFloatingId`).

## Example Usage

```hcl
data "viettelidc_floating_ip" "fip" {
  id     = "existing-fip-id"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Required) Floating IP ID (`vttFloatingId`).
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `public_ip` - Public IPv4 address.
* `instance_id` - Associated VM Instance ID.
* `network_interface_id` - Associated NIC ID.
