data "viettelidc_ovpc_instance" "vm" {
  name   = "web-server"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
