data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# 1. Discover HTTP listener by Load Balancer name and port (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

# 2. Discover backend pool by Load Balancer name and pool name
data "viettelidc_ovpc_lb_pool" "api" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  name               = "api-backend-pool"
}

# Example 1: Reject malicious or unwanted traffic
resource "viettelidc_ovpc_lb_l7_policy" "reject_bots" {
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  listener_id = data.viettelidc_ovpc_lb_listener.http.id
  name        = "reject-bots"
  description = "Block scrapers and scanners"
  action      = "REJECT"
  priority    = 1
}

# Example 2: Redirect legacy path or domain to HTTPS endpoint
resource "viettelidc_ovpc_lb_l7_policy" "redirect_portal" {
  vpc_id       = data.viettelidc_ovpc_vpc.main.id
  listener_id  = data.viettelidc_ovpc_lb_listener.http.id
  name         = "redirect-portal"
  description  = "Redirect old portal users to new domain"
  action       = "REDIRECT_TO_URL"
  redirect_url = "https://portal.viettelidc.com.vn"
  priority     = 10
}

# Example 3: Route specific traffic to dedicated application pool
resource "viettelidc_ovpc_lb_l7_policy" "route_to_api_pool" {
  vpc_id           = data.viettelidc_ovpc_vpc.main.id
  listener_id      = data.viettelidc_ovpc_lb_listener.http.id
  name             = "route-api"
  description      = "Direct /api/* requests to microservice pool"
  action           = "REDIRECT_TO_POOL"
  redirect_pool_id = data.viettelidc_ovpc_lb_pool.api.id
  priority         = 20
}
