data "viettelidc_ovpc_load_balancer" "web" {
  name   = "web-lb"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
