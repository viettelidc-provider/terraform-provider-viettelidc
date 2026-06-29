# Cáº¥u hÃ¬nh Lá»‹ch sao lÆ°u tá»± Ä‘á»™ng hÃ ng thÃ¡ng
resource "viettelidc_vdbs_backup_scheduler" "monthly_backup" {
  name           = "uc4-db-backup-3"
  description    = "Monthly backup scheduler created via Terraform"
  scheduler_type = "monthly"
  location       = "block"
  max_record     = 3
  instance_id    = "2851"
  time           = "2026-07-01 01:00:00"
}
