---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_load_balancer (Data Source)"
sidebar_current: "docs-viettelidc-datasource-load-balancer"
description: |-
  Retrieves information about an existing ViettelIDC Load Balancer.
---

# Data Source: viettelidc\_load\_balancer

Use this data source to look up a Load Balancer by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_load_balancer" "web" {
  name   = "web-lb"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Load Balancer ID. Either `id` or `name` must be set.
* `name` - (Optional) Load Balancer name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Load Balancer ID.
* `name` - Name.
* `description` - Description.
* `subnet_id` - Subnet ID.
* `floating_ip_id` - Floating IP ID.
* `loadbalancer_type` - Load Balancer type.
* `package_type` - Package type.
* `admin_state_up` - Administrative state.
* `status` - Provisioning status.
* `operating_status` - Operating status.
* `listeners` - List of Listeners. Each item exports: `id`, `name`, `description`, `protocol`, `protocol_port`, `x_forwarded_for`, `x_forwarded_port`, `x_forwarded_proto`.
* `pools` - List of Pools. Each item exports: `id`, `name`, `description`, `algorithm`, `session_persistence_type`.
