---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_load_balancer"
sidebar_current: "docs-viettelidc-resource-load-balancer"
description: |-
  Provides a ViettelIDC Load Balancer.
---

# viettelidc\_ovpc\_load\_balancer

Provides a Load Balancer for distributing traffic across multiple VM Instances on ViettelIDC. The resource automatically creates a Listener and Pool based on the `loadbalancer_type` configuration.

## Example Usage

```hcl
resource "viettelidc_ovpc_load_balancer" "web" {
  name              = "web-lb"
  description       = "Load balancer for web tier"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  floating_ip_id    = viettelidc_ovpc_floating_ip.fip.id
  loadbalancer_type = "APPLICATION HTTP-HTTPS"
  package_type      = "LB Compact"
  vpc_id            = viettelidc_ovpc_vpc.main.id
  admin_state_up    = true

  pool_members {
    vm_id  = viettelidc_ovpc_instance.vm1.id
    port   = 80
    weight = 1
  }

  pool_members {
    vm_id  = viettelidc_ovpc_instance.vm2.id
    port   = 80
    weight = 1
  }
}
```

## Argument Reference

* `name` - (Required) Human-readable Load Balancer name.
* `subnet_id` - (Required, ForceNew) Subnet ID where the Load Balancer will be placed. **Cannot be changed after creation.**
* `loadbalancer_type` - (Required, ForceNew) Load Balancer type (e.g. `"APPLICATION HTTP-HTTPS"`). **Cannot be changed after creation.**
* `package_type` - (Required, ForceNew) Load Balancer package (e.g. `"LB Compact"`). **Cannot be changed after creation.**
* `description` - (Optional) Description.
* `floating_ip_id` - (Optional) Floating IP ID to assign to the Load Balancer.
* `admin_state_up` - (Optional) Administrative state. Defaults to `true`.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.
* `pool_members` - (Optional) List of pool members. Each member supports:
  * `vm_id` - (Required) VM Instance ID.
  * `port` - (Required) Port on the VM. Defaults to `80`.
  * `weight` - (Optional) Member weight. Defaults to `1`.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Load Balancer ID (`vttLoadBalancerId`).
* `status` - Provisioning status.
* `operating_status` - Operating status.
* `listeners` - List of Listeners. Each listener has: `id`, `name`, `description`, `protocol`, `protocol_port`, `x_forwarded_for`, `x_forwarded_port`, `x_forwarded_proto`.
* `pools` - List of Pools. Each pool has: `id`, `name`, `description`, `algorithm`, `session_persistence_type`.

## Import

Load Balancers can be imported using the Load Balancer ID:

```
terraform import viettelidc_ovpc_load_balancer.web <load_balancer_id>
```
