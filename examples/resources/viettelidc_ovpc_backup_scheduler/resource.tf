# The backup service identifies VMs by its own UUIDs, not by the numeric
# instance id. Look them up instead of hardcoding.
data "viettelidc_ovpc_backup_vms" "available" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_backup_scheduler" "daily" {
  name             = "daily-vm-backup"
  description      = "Every day at 04:00"
  cycle            = "DAILY"
  start_date       = "2026-09-01"
  start_time       = "04:00:00"
  number_of_record = 7

  # Pick by VM name; adding or removing entries updates the schedule in place.
  vm_ids = [
    for vm in data.viettelidc_ovpc_backup_vms.available.vms :
    vm.id if vm.name == "my-app-server"
  ]

  # Destroying the schedule keeps its backup records unless this is true.
  delete_records_on_destroy = false

  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
