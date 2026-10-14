data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Resolve listener dynamically using LB name and port (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

# List all L7 policies attached to the resolved listener
data "viettelidc_ovpc_lb_l7_policies" "web_listener_policies" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
}

output "policies" {
  value = [
    for p in data.viettelidc_ovpc_lb_l7_policies.web_listener_policies.policies : {
      id        = p.id
      name      = p.name
      action    = p.action
      pool_name = p.pool_name
      priority  = p.priority
      status    = p.status
    }
  ]
}
