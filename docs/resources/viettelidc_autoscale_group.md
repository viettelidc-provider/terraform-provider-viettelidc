---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_autoscale_group"
sidebar_current: "docs-viettelidc-resource-autoscale-group"
description: |-
  Provides a ViettelIDC Autoscale Group.
---

# viettelidc\_autoscale\_group

Provides an Autoscale Group on ViettelIDC. An Autoscale Group automatically scales VM Instances up or down based on CPU utilisation thresholds.

~> **Note:** All attributes are immutable (ForceNew). Any change destroys and recreates the entire Autoscale Group. The API has no detail endpoint for ASGs — reads are performed via list+filter.

## Example Usage

```hcl
resource "viettelidc_autoscale_group" "web" {
  name                = "web-asg"
  launch_template_id  = viettelidc_launch_template.web.id
  is_autoscale        = true
  desired_capacity    = 2
  min_size            = 1
  max_size            = 5
  metric_type         = "CPU"
  scale_out_threshold = 80
  scale_in_threshold  = 20
  has_load_balancer   = false
  vpc_id              = viettelidc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required, ForceNew) Autoscale Group name.
* `launch_template_id` - (Required, ForceNew) ID of the Launch Template to use.
* `is_autoscale` - (Required, ForceNew) Whether automatic scaling is enabled.
* `desired_capacity` - (Required, ForceNew) Desired number of instances.
* `min_size` - (Required, ForceNew) Minimum number of instances.
* `max_size` - (Required, ForceNew) Maximum number of instances.
* `metric_type` - (Optional, ForceNew) Scaling metric type. Defaults to `"CPU"`.
* `scale_out_threshold` - (Optional, ForceNew) CPU percentage threshold to trigger scale-out.
* `scale_in_threshold` - (Optional, ForceNew) CPU percentage threshold to trigger scale-in.
* `has_load_balancer` - (Optional, ForceNew) Whether the ASG is attached to a Load Balancer. Defaults to `false`.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Autoscale Group ID.

## Import

Autoscale Groups can be imported using the Autoscale Group ID:

```
terraform import viettelidc_autoscale_group.web <autoscale_group_id>
```
