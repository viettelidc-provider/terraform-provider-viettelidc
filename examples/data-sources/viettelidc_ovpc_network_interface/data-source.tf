data "viettelidc_ovpc_network_interface" "nic" {
  name   = "my-nic"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

# Look up by private IP address
data "viettelidc_ovpc_network_interface" "by_ip" {
  ip_address = "10.0.1.15"
  vpc_id     = data.viettelidc_ovpc_vpc.main.id
}
