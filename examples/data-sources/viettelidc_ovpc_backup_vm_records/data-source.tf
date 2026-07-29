# Every VM backup the VPC holds, newest first.
data "viettelidc_ovpc_backup_vm_records" "all" {
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

# Records a given schedule produced. scheduleName is empty for manual backups,
# so filtering on it is also how you tell the two apart.
output "daily_backups" {
  value = [
    for r in data.viettelidc_ovpc_backup_vm_records.all.records :
    { name = r.name, size = r.size, at = r.created_date }
    if r.schedule_name == viettelidc_ovpc_backup_scheduler.daily.name
  ]
}
