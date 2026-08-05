# auto: system assigns the IP
resource "viettelidc_ovpc_network_interface" "nic" {
  name           = "my-nic"
  subnet_id      = viettelidc_ovpc_subnet.private.id
  ip_assign_type = "auto"
  vpc_id         = data.viettelidc_ovpc_vpc.main.id
}

# custom: you specify the IP
resource "viettelidc_ovpc_network_interface" "nic_custom" {
  name           = "my-nic-custom"
  subnet_id      = viettelidc_ovpc_subnet.private.id
  ip_assign_type = "custom"
  ip_address     = "10.21.10.1"
  vpc_id         = data.viettelidc_ovpc_vpc.main.id
}
