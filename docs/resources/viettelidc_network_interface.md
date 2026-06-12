---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_network_interface"
sidebar_current: "docs-viettelidc-resource-network-interface"
description: |-
  Provides a ViettelIDC Network Interface (NIC).
---

# viettelidc\_network\_interface

Provides a Network Interface (NIC) resource on ViettelIDC. A NIC can be attached to an Instance or used to receive a Floating IP.

## Example Usage (Auto-assigned IP)

```hcl
resource "viettelidc_network_interface" "nic" {
  name           = "my-nic"
  subnet_id      = viettelidc_subnet.private.id
  ip_assign_type = "auto"
  vpc_id         = viettelidc_vpc.main.id
}
```

## Example Usage (Static IP)

```hcl
resource "viettelidc_network_interface" "nic_static" {
  name           = "my-nic-static"
  subnet_id      = viettelidc_subnet.private.id
  ip_assign_type = "STATIC"
  ip_address     = "10.0.1.50"
  vpc_id         = viettelidc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Human-readable NIC name.
* `subnet_id` - (Required) Subnet ID to attach the NIC to.
* `ip_assign_type` - (Required, ForceNew) IP assignment type: `STATIC` or `auto`. **Cannot be changed after creation.**
* `ip_address` - (Optional) IP address. Required when `ip_assign_type = "STATIC"`; assigned by the system when `auto`. Must not be set when `ip_assign_type = "auto"`.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.
* `description` - (Optional) Description of the NIC.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - NIC ID (`vttNetworkInterfaceId`).
* `status` - Current status of the NIC.

## Import

Network Interfaces can be imported using the NIC ID:

```
terraform import viettelidc_network_interface.nic <network_interface_id>
```
