data "viettelidc_ovpc_floating_ip" "fip" {
  public_ip = "1.2.3.4"
  vpc_id    = data.viettelidc_ovpc_vpc.main.id
}

# Or look up by name
data "viettelidc_ovpc_floating_ip" "by_name" {
  name   = "web-fip"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
