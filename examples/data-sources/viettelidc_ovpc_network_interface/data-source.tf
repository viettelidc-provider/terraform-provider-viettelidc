data "viettelidc_ovpc_network_interface" "nic" {
  id     = "nic-id"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
