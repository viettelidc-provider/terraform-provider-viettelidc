# Cáº¥u hÃ¬nh Backup Scheduler cho Volume Block Storage
resource "viettelidc_vks_scheduler" "block_backup_scheduler" {
  name        = "uc5-block-backup-schedule-3"
  start_time  = "2026-07-01 02:00:00"
  finish_time = "2026-07-02 06:00:00"
  cycle       = 86400
  unit        = "day"
  quantity    = 1
  cluster_id  = 1477
  volume_ids  = [255]
  host_id     = 6
}
