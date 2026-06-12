---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_launch_templates (Data Source)"
sidebar_current: "docs-viettelidc-datasource-launch-templates"
description: |-
  Retrieves all Launch Templates in a ViettelIDC VPC.
---

# Data Source: viettelidc\_launch\_templates

Use this data source to list all Launch Templates in a VPC.

## Example Usage

```hcl
data "viettelidc_launch_templates" "all" {
  vpc_id = viettelidc_vpc.main.id
}

output "template_names" {
  value = [for t in data.viettelidc_launch_templates.all.launch_templates : t.name]
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID filter. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `launch_templates` - List of Launch Templates. Each item exports:
  * `id` - Launch Template ID.
  * `name` - Template name.
  * `description` - Description.
  * `vm_id` - Source VM ID.
  * `memory_size` - Memory in GB.
  * `cpu_size` - Number of vCPUs.
