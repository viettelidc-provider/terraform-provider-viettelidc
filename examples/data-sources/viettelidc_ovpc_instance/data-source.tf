data "viettelidc_ovpc_instance" "vm" {
  id     = "vm-id"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
