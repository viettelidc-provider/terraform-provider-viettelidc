resource "viettelidc_ovpc_backup_plan" "daily" {
  name             = "daily-backup"
  description      = "Daily backup at 04:00"
  cycle            = "DAILY"
  start_date       = "2026-09-01"
  start_time       = "04:00:00"
  number_of_record = 7

  # Numeric instance ids are resolved to the backup service's VM UUIDs.
  # A UUID can also be given directly.
  vm_ids = [viettelidc_ovpc_instance.vm.id]

  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
