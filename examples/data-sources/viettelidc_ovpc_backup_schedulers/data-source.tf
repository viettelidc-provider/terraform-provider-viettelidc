data "viettelidc_ovpc_backup_schedulers" "all" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

output "schedule_ids" {
  value = { for s in data.viettelidc_ovpc_backup_schedulers.all.schedulers : s.name => s.id }
}
