data "viettelidc_ovpc_floating_ip" "fip" {
  public_ip = "1.2.3.4"
  vpc_id    = viettelidc_ovpc_vpc.main.id
}
