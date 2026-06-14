---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_volume"
sidebar_current: "docs-viettelidc-resource-volume"
description: |-
  Provides a ViettelIDC Block Storage Volume.
---

# viettelidc\_ovpc\_volume

Provides a Block Storage Volume on ViettelIDC. Use `viettelidc_ovpc_volume_attachment` to attach the volume to an instance after creation.

## Example Usage

```hcl
resource "viettelidc_ovpc_volume" "data" {
  name        = "data-disk"
  size        = 100
  volume_type = "SSD"
  vpc_id      = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_volume_attachment" "attach" {
  instance_id = viettelidc_ovpc_instance.vm.id
  volume_id   = viettelidc_ovpc_volume.data.id
  vpc_id      = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Volume name.
* `size` - (Required) Volume size in GiB.
* `volume_type` - (Optional, ForceNew) Volume type: `"SSD"` or `"HDD"`. **Cannot be changed after creation.**
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Volume ID.
* `status` - Volume status (`AVAILABLE`, `IN-USE`, ...).

## Import

Volumes can be imported using the Volume ID:

```
terraform import viettelidc_ovpc_volume.data <volume_id>
```
