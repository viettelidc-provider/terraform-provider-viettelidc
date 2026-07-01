data "viettelidc_ovpc_network_interfaces" "all" {
  vpc_id = viettelidc_ovpc_vpc.main.id
}
