---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_vfirewalls (Data Source)"
sidebar_current: "docs-viettelidc-datasource-vfirewalls"
description: |-
  Retrieves all vFirewall instances in a ViettelIDC VPC.
---

# Data Source: viettelidc\_vfirewalls

Use this data source to list all vFirewall instances in a VPC. Only list is supported — the API does not provide create or delete endpoints for vFirewalls.

## Example Usage

```hcl
data "viettelidc_vfirewalls" "all" {
  vpc_id = viettelidc_vpc.main.id
}

output "firewall_ids" {
  value = [for fw in data.viettelidc_vfirewalls.all.items : fw.id]
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID filter. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `items` - List of vFirewall instances. Each item exports:
  * `id` - vFirewall ID.
  * `name` - vFirewall name.
  * `status` - Power state (`POWERED_ON`, `POWERED_OFF`, ...).
  * `availability_zone` - Availability zone.
  * `cpu` - Number of vCPUs.
  * `memory` - RAM in MB.
  * `external_ip` - External IP address.
  * `created_at` - Creation timestamp.
