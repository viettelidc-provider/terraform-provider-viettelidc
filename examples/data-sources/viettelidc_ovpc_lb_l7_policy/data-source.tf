data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Resolve listener dynamically using LB name and port
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

# Look up L7 policy by name on the resolved listener
data "viettelidc_ovpc_lb_l7_policy" "dev_policy" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
  name        = "dev"
}

output "policy_info" {
  value = {
    id           = data.viettelidc_ovpc_lb_l7_policy.dev_policy.id
    name         = data.viettelidc_ovpc_lb_l7_policy.dev_policy.name
    action       = data.viettelidc_ovpc_lb_l7_policy.dev_policy.action
    redirect_url = data.viettelidc_ovpc_lb_l7_policy.dev_policy.redirect_url
  }
}
