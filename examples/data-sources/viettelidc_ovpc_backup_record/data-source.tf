data "viettelidc_ovpc_backup_record" "latest" {
  backup_plan_id = viettelidc_ovpc_backup_plan.daily.id
  vpc_id         = viettelidc_ovpc_vpc.main.id
}
