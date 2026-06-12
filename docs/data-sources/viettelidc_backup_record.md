---
layout: "vcloud"
page_title: "Viettel IDC Cloud: viettelidc_backup_record (Data Source)"
sidebar_current: "docs-viettelidc-datasource-backup-record"
description: |-
  Retrieves information about a ViettelIDC Backup Record.
---

# Data Source: viettelidc\_backup\_record

Use this data source to look up a Backup Record by `id` or `backup_plan_id`.

## Example Usage

```hcl
data "viettelidc_backup_record" "latest" {
  backup_plan_id = viettelidc_backup_plan.daily.id
  vpc_id         = viettelidc_vpc.main.id
}
```

## Argument Reference

* `id` - (Optional) Backup Record ID.
* `backup_plan_id` - (Optional) Filter by Backup Plan ID.
* `vpc_id` - (Optional) VPC ID. Falls back to the provider default `vpc_id` when unset.

## Attributes Reference

* `id` - Backup Record ID.
* `volume_id` - ID of the volume that was backed up.
* `volume_name` - Name of the volume.
* `volume_size` - Size of the volume in GiB.
* `status` - Status of the backup record.
* `created_at` - Timestamp when the backup was created.
