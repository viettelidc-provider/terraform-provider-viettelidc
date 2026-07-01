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
