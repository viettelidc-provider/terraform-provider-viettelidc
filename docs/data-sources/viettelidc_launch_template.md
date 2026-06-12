---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_launch_template (Data Source)"
sidebar_current: "docs-viettelidc-datasource-launch-template"
description: |-
  Retrieves information about an existing ViettelIDC Launch Template.
---

# Data Source: viettelidc\_launch\_template

Use this data source to look up a Launch Template by `id` or `name`.

## Example Usage

```hcl
data "viettelidc_launch_template" "web" {
  name   = "web-template"
  vpc_id = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Launch Template ID. Either `id` or `name` must be set.
* `name` - (Optional) Template name. Either `id` or `name` must be set.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Launch Template ID.
* `name` - Template name.
* `description` - Description.
* `vm_id` - Source VM ID.
* `memory_size` - Memory in GB.
* `cpu_size` - Number of vCPUs.
