data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Resolve listener dynamically using LB name and port (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

# List available backend pools compatible with REDIRECT_TO_POOL on this listener
data "viettelidc_ovpc_lb_l7_available_pools" "pools" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
}

output "available_pools" {
  value = [
    for p in data.viettelidc_ovpc_lb_l7_available_pools.pools.pools : {
      id   = p.id
      name = p.name
    }
  ]
}
