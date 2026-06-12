---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_autoscale_groups (Data Source)"
sidebar_current: "docs-viettelidc-datasource-autoscale-groups"
description: |-
  Retrieves all Autoscale Groups in a ViettelIDC VPC.
---

# Data Source: viettelidc\_autoscale\_groups

Use this data source to list all Autoscale Groups in a VPC. The API has no detail endpoint for ASGs — only list is supported.

## Example Usage

```hcl
data "viettelidc_autoscale_groups" "all" {
  vpc_id = viettelidc_vpc.main.id
}

output "asg_names" {
  value = [for g in data.viettelidc_autoscale_groups.all.autoscale_groups : g.name]
}
```

## Argument Reference

* `vpc_id` - (Optional) VPC ID filter. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `autoscale_groups` - List of Autoscale Groups. Each item exports:
  * `id` - Autoscale Group ID.
  * `name` - Name.
  * `launch_template_id` - Launch Template ID.
  * `is_autoscale` - Whether auto-scaling is enabled.
  * `desired_capacity` - Desired instance count.
  * `min_size` - Minimum instance count.
  * `max_size` - Maximum instance count.
  * `metric_type` - Scaling metric type.
  * `scale_out_threshold` - Scale-out CPU percentage threshold.
  * `scale_in_threshold` - Scale-in CPU percentage threshold.
  * `has_load_balancer` - Whether attached to a Load Balancer.
