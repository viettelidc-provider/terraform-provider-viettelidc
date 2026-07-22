# Every VM the backup service accepts for this VPC.
data "viettelidc_ovpc_backup_vms" "all" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

# Only the ones not already covered by a schedule.
data "viettelidc_ovpc_backup_vms" "unassigned" {
  filter = "unassigned"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

output "backup_vm_uuid_by_name" {
  value = { for vm in data.viettelidc_ovpc_backup_vms.all.vms : vm.name => vm.id }
}
