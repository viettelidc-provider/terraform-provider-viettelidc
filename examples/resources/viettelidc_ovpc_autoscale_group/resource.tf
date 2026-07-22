# The API accepts two shapes and they are not interchangeable.

# 1. Autoscale mode — the group grows and shrinks on a metric.
resource "viettelidc_ovpc_autoscale_group" "web" {
  name                = "web-asg"
  launch_template_id  = viettelidc_ovpc_launch_template.web.id
  is_autoscale        = true
  desired_capacity    = 2
  min_size            = 1
  max_size            = 5
  metric_type         = "CPU"
  scale_out_threshold = 80
  scale_in_threshold  = 20
  has_load_balancer   = false
  vpc_id              = data.viettelidc_ovpc_vpc.main.id
}

# 2. Load-balancer mode — a fixed pool registered behind an existing load
# balancer. min_size / max_size / the thresholds do not apply here and are not
# sent; supplying them is a config error.
resource "viettelidc_ovpc_autoscale_group" "fixed_pool" {
  name               = "web-pool"
  launch_template_id = viettelidc_ovpc_launch_template.web.id
  is_autoscale       = false
  desired_capacity   = 1

  has_load_balancer    = true
  loadbalancer_id      = viettelidc_ovpc_load_balancer.web.id
  loadbalancer_pool_id = viettelidc_ovpc_load_balancer.web.pools[0].id
  subnet_id            = viettelidc_ovpc_subnet.public.id
  port_number          = 80

  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
