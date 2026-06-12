---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_instance"
sidebar_current: "docs-viettelidc-resource-instance"
description: |-
  Provides a ViettelIDC Compute Instance (VM).
---

# viettelidc\_instance

Provides a Compute Instance (virtual machine) on ViettelIDC. The resource polls until the instance reaches `POWERED_ON` or `ACTIVE` status (up to 25 minutes).

## Example Usage

```hcl
resource "viettelidc_instance" "vm" {
  template_id        = 101
  subnet_id          = viettelidc_subnet.private.id
  admin_pass         = "MySecretPass123!"
  cpu                = 2
  memory             = 4096
  storage_type       = "SSD"
  key_pair_name      = viettelidc_key_pair.deploy.key_name
  security_group_ids = [viettelidc_security_group.web.id]
  availability_zone  = "HN1"
  vpc_id             = viettelidc_vpc.main.id
}
```

## Argument Reference

* `template_id` - (Required, ForceNew) Integer ID of the VM template (image).
* `subnet_id` - (Required, ForceNew) Subnet ID to attach the primary NIC to. **Cannot be changed after creation.**
* `instance_type_id` - (Optional) Integer ID of the instance type (CPU/RAM package).
* `admin_pass` - (Optional, Sensitive) Initial admin password.
* `cpu` - (Optional) Number of vCPUs. Can be changed (resize) — Terraform stops the instance, resizes, then starts it again.
* `memory` - (Optional) RAM in MB. Can be changed (resize).
* `storage_type` - (Optional, ForceNew) Root volume storage type: `"SSD"` or `"HDD"`. Defaults to `"HDD"`.
* `key_pair_name` - (Optional, ForceNew) Key pair name to inject into the instance.
* `security_group_ids` - (Optional) List of Security Group IDs to attach.
* `availability_zone` - (Optional, ForceNew) Availability zone.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Instance ID (`vttVmId`).
* `name` - Instance name assigned by the platform.
* `status` - Instance status (`POWERED_ON`, `ACTIVE`, `POWERED_OFF`, ...).
* `ip_address` - IP address of the primary NIC.
* `root_nic_id` - ID of the primary NIC auto-created with this instance. Use this when associating a Floating IP without creating a separate NIC.
* `image_id` - ID of the image used.
* `image_name` - Name of the image used.

## Import

Instances can be imported using the Instance ID:

```
terraform import viettelidc_instance.vm <instance_id>
```
