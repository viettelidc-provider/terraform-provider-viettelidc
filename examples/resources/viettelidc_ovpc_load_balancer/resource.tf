resource "viettelidc_ovpc_load_balancer" "web" {
  name              = "web-lb"
  description       = "Load balancer for web tier"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  floating_ip_id    = viettelidc_ovpc_floating_ip.fip.id
  loadbalancer_type = "APPLICATION HTTP-HTTPS"
  package_type      = "LB Compact"
  vpc_id            = viettelidc_ovpc_vpc.main.id
  admin_state_up    = true

  pool_members {
    vm_id  = viettelidc_ovpc_instance.vm1.id
    port   = 80
    weight = 1
  }

  pool_members {
    vm_id  = viettelidc_ovpc_instance.vm2.id
    port   = 80
    weight = 1
  }
}
