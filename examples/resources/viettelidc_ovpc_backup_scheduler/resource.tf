# VM-level backup schedule.
# vm_ids are the backup service's VM UUIDs, which are not the numeric instance
# ids: list them with GET /backup/api/v1/vpc/{vpc_id}/vms/all.
resource "viettelidc_ovpc_backup_scheduler" "daily" {
  name             = "daily-vm-backup"
  description      = "Every day at 04:00"
  cycle            = "DAILY"
  start_date       = "2026-09-01"
  start_time       = "04:00:00"
  number_of_record = 7

  vm_ids = [
    "78ec434d-8cc0-45c8-9bb6-8c3435a7f567",
  ]

  # Destroying the schedule keeps the backup records unless this is true.
  delete_records_on_destroy = false

  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
