data "viettelidc_ovpc_vfirewalls" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}

output "firewall_ids" {
  value = [for fw in data.viettelidc_ovpc_vfirewalls.all.items : fw.id]
}
