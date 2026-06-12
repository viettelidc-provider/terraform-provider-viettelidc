---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_network_interface_attachment"
sidebar_current: "docs-viettelidc-resource-network-interface-attachment"
description: |-
  Attaches a ViettelIDC Network Interface (NIC) to a VM Instance.
---

# viettelidc\_network\_interface\_attachment

Attaches an existing Network Interface (NIC) to a VM Instance on ViettelIDC.

## Example Usage

```hcl
resource "viettelidc_network_interface_attachment" "attach" {
  network_interface_id = viettelidc_network_interface.nic.id
  instance_id          = viettelidc_instance.vm.id
  vpc_id               = viettelidc_vpc.main.id
}
```

## Argument Reference

* `network_interface_id` - (Required, ForceNew) ID of the NIC to attach.
* `instance_id` - (Required, ForceNew) ID of the VM Instance to receive the NIC.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Composite ID in the format `<network_interface_id>/<instance_id>`.

## Import

NIC Attachments can be imported using the composite ID:

```
terraform import viettelidc_network_interface_attachment.attach <network_interface_id>/<instance_id>
```
