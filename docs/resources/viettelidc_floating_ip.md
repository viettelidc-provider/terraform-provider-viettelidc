---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_floating_ip"
sidebar_current: "docs-viettelidc-resource-floating-ip"
description: |-
  Allocates and associates a ViettelIDC Floating IP with a VM Instance.
---

# viettelidc\_floating\_ip

Allocates a Floating IP (public IP address) and associates it with a VM Instance and NIC on ViettelIDC. Destroying the resource disassociates and releases the IP back to the pool.

## Example Usage (Allocate new)

```hcl
resource "viettelidc_floating_ip" "fip" {
  instance_id          = viettelidc_instance.vm.id
  network_interface_id = viettelidc_instance.vm.root_nic_id
  vpc_id               = viettelidc_vpc.main.id
}
```

## Example Usage (Use existing Floating IP)

```hcl
resource "viettelidc_floating_ip" "fip_existing" {
  id                   = "existing-fip-id"
  instance_id          = viettelidc_instance.vm.id
  network_interface_id = viettelidc_instance.vm.root_nic_id
  vpc_id               = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Floating IP ID (`vttFloatingId`). When set, the resource associates this existing FIP instead of allocating a new one.
* `instance_id` - (Optional, ForceNew) VM Instance ID to associate with. Changing this forces a new resource.
* `network_interface_id` - (Optional, ForceNew) NIC ID to associate with. Changing this forces a new resource.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `public_ip` - Public IPv4 address allocated by the system.

## Import

Floating IPs can be imported using the Floating IP ID:

```
terraform import viettelidc_floating_ip.fip <floating_ip_id>
```
