data "viettelidc_ovpc_floating_ip" "fip" {
  id     = "existing-fip-id"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
