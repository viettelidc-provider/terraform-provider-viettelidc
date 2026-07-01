data "viettelidc_ovpc_subnets" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}

output "subnet_ids" {
  value = [for s in data.viettelidc_ovpc_subnets.all.subnets : s.id]
}
