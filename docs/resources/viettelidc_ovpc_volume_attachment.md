---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_volume_attachment"
sidebar_current: "docs-viettelidc-resource-volume-attachment"
description: |-
  Attaches a ViettelIDC Block Storage Volume to a Compute Instance.
---

# viettelidc\_ovpc\_volume\_attachment

Attaches a Block Storage Volume to a Compute Instance on ViettelIDC.

## Example Usage

```hcl
resource "viettelidc_ovpc_volume_attachment" "attach" {
  instance_id = viettelidc_ovpc_instance.vm.id
  volume_id   = viettelidc_ovpc_volume.data.id
  vpc_id      = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `instance_id` - (Required, ForceNew) ID of the Instance to attach to.
* `volume_id` - (Required, ForceNew) ID of the Volume to attach.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Composite ID in the format `<instance_id>/<volume_id>`.

## Import

Volume Attachments can be imported using the composite ID:

```
terraform import viettelidc_ovpc_volume_attachment.attach <instance_id>/<volume_id>
```
