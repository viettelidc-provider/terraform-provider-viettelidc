data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

# Look up backend pool using load balancer name and pool name (zero hardcoded IDs!)
data "viettelidc_ovpc_lb_pool" "backend_pool" {
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
  load_balancer_name = "web-lb"
  name               = "web-backend-pool"
}

output "pool_info" {
  value = {
    id        = data.viettelidc_ovpc_lb_pool.backend_pool.id
    name      = data.viettelidc_ovpc_lb_pool.backend_pool.name
    algorithm = data.viettelidc_ovpc_lb_pool.backend_pool.algorithm
  }
}
