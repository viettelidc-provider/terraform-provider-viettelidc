data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

data "viettelidc_ovpc_load_balancer" "web" {
  name   = "web-lb"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

output "load_balancer_id" {
  value = data.viettelidc_ovpc_load_balancer.web.load_balancer_id
}
