data "viettelidc_ovpc_autoscale_groups" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}

output "asg_names" {
  value = [for g in data.viettelidc_ovpc_autoscale_groups.all.autoscale_groups : g.name]
}
