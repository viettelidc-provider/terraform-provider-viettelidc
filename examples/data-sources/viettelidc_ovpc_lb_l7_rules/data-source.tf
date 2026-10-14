data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Resolve listener dynamically using LB name and port (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

# Look up policy dynamically by name on that listener
data "viettelidc_ovpc_lb_l7_policy" "api" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
  name        = "route-api"
}

# List all rules belonging to this policy
data "viettelidc_ovpc_lb_l7_rules" "policy_rules" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
  policy_id   = data.viettelidc_ovpc_lb_l7_policy.api.id
}

output "rules" {
  value = [
    for r in data.viettelidc_ovpc_lb_l7_rules.policy_rules.rules : {
      id           = r.id
      type         = r.type
      compare_type = r.compare_type
      key          = r.key
      value        = r.value
      status       = r.status
    }
  ]
}
