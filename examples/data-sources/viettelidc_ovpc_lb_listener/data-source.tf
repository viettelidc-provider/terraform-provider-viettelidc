data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Look up listener using load balancer name and port (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_listener" "http" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  port               = 80
}

output "listener_info" {
  value = {
    id       = data.viettelidc_ovpc_lb_listener.http.id
    name     = data.viettelidc_ovpc_lb_listener.http.name
    port     = data.viettelidc_ovpc_lb_listener.http.port
    protocol = data.viettelidc_ovpc_lb_listener.http.protocol
  }
}
