resource "viettelidc_ovpc_network_interface" "nic" {
  name           = "my-nic"
  subnet_id      = viettelidc_ovpc_subnet.private.id
  ip_assign_type = "auto"
  vpc_id         = data.viettelidc_ovpc_vpc.main.id
}
