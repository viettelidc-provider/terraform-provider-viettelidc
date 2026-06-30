---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_ovpc_backup_plan"
sidebar_current: "docs-viettelidc-resource-backup-plan"
description: |-
  Provides a ViettelIDC Backup Plan for scheduling volume backups.
---

# viettelidc\_ovpc\_backup\_plan

Provides a Backup Plan for automatically backing up Block Storage Volumes on a schedule on ViettelIDC.

## Example Usage

```hcl
resource "viettelidc_ovpc_backup_plan" "daily" {
  name             = "daily-backup"
  description      = "Daily backup at 2 AM"
  backup_cycle_id  = 1
  start_day_backup = "2024-01-01"
  time_backup      = "02:00:00"
  number_of_record = 7
  volume_ids       = [viettelidc_ovpc_volume.data.id]
  vpc_id           = viettelidc_ovpc_vpc.main.id
}
```

## Argument Reference

* `name` - (Required) Human-readable Backup Plan name.
* `backup_cycle_id` - (Required) ID of the backup cycle (e.g. `1` = daily, `2` = weekly).
* `start_day_backup` - (Required) Start date for the backup schedule in `YYYY-MM-DD` format.
* `time_backup` - (Required) Time for the backup in `HH:MM:SS` format.
* `number_of_record` - (Required) Number of backup records to retain.
* `volume_ids` - (Required) List of Volume IDs to include in the backup plan.
* `description` - (Optional) Description.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - Backup Plan ID.
* `status` - Current status of the Backup Plan.
* `backup_cycle_name` - Name of the backup cycle.

## Import

Backup Plans can be imported using the Backup Plan ID:

```
terraform import viettelidc_ovpc_backup_plan.daily <backup_plan_id>
```
