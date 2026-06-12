---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_launch_template"
sidebar_current: "docs-viettelidc-resource-launch-template"
description: |-
  Provides a ViettelIDC Launch Template for Autoscale Groups.
---

# viettelidc\_launch\_template

Provides a Launch Template for use with Autoscale Groups on ViettelIDC. A Launch Template defines the VM configuration (CPU, RAM, source VM) that an Autoscale Group uses when creating new instances.

~> **Note:** All attributes are immutable. Any change destroys the old template and creates a new one.

## Example Usage

```hcl
resource "viettelidc_launch_template" "web" {
  name        = "web-template"
  description = "Template for web servers"
  vm_id       = viettelidc_instance.base_vm.id
  memory_size = 4
  cpu_size    = 2
  vpc_id      = viettelidc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required, ForceNew) Launch Template name.
* `vm_id` - (Required, ForceNew) Source VM ID used to seed the template.
* `memory_size` - (Required, ForceNew) Memory size in GB.
* `cpu_size` - (Required, ForceNew) Number of vCPUs.
* `description` - (Optional, ForceNew) Description.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Launch Template ID.

## Import

Launch Templates can be imported using the Launch Template ID:

```
terraform import viettelidc_launch_template.web <launch_template_id>
```
