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
  vpc_id              = viettelidc_ovpc_vpc.main.id
}
