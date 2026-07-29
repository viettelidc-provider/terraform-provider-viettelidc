resource "viettelidc_ovpc_load_balancer" "web" {
  name              = "web-lb"
  description       = "Load balancer for web tier"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  floating_ip_id    = viettelidc_ovpc_floating_ip.fip.id
  loadbalancer_type = "APPLICATION HTTP-HTTPS"
  package_type      = "LB Compact"
  vpc_id            = data.viettelidc_ovpc_vpc.main.id
  admin_state_up    = true

  # pool_members is an attribute list, not a block - note the `=` and the brackets.
  pool_members = [
    {
      vm_id  = viettelidc_ovpc_instance.vm1.id
      port   = 80
      weight = 1
    },
    {
      vm_id  = viettelidc_ovpc_instance.vm2.id
      port   = 80
      weight = 1
    },
  ]
}

# The listener, pool and health check are created along with the load balancer.
# Left out they default to HTTP:80, ROUND_ROBIN and an HTTP GET / check, which
# is wrong for anything that is not a web tier.
resource "viettelidc_ovpc_load_balancer" "tcp" {
  name              = "db-lb"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  loadbalancer_type = "NETWORK TCP-UDP"
  package_type      = "LB Large"
  vpc_id            = data.viettelidc_ovpc_vpc.main.id

  listener_protocol        = "TCP"
  listener_port            = 5432
  pool_algorithm           = "LEAST_CONNECTIONS"
  pool_session_persistence = "SOURCE_IP"

  # Not every listener protocol accepts every check; the API rejects the bad
  # pairs with LOADBALANCER_MONITOR_AND_POOL_NOT_VALID_PROTOCOL.
  monitor_type  = "PING"
  monitor_delay = 10
}

# 3. HTTPS with TLS terminated at the load balancer. The listener protocol must
# be TERMINATED_HTTPS and certificate_id points at a viettelidc_ovpc_certificate.
resource "viettelidc_ovpc_load_balancer" "https" {
  name              = "web-https"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  loadbalancer_type = "APPLICATION HTTP-HTTPS"
  package_type      = "LB Large"
  vpc_id            = data.viettelidc_ovpc_vpc.main.id

  listener_protocol = "TERMINATED_HTTPS"
  listener_port     = 443
  certificate_id    = viettelidc_ovpc_certificate.web_cert.id

  pool_members = [{
    vm_id  = viettelidc_ovpc_instance.vm1.id
    port   = 443
    weight = 1
  }]
}

# 4. Attach an extra TLS-terminating listener to a load balancer after the fact.
# Each additional_listener block is its own listener + pool + members + monitor,
# created through the same compound-create the console uses for its "attach
# certificate" flow. Removing a block deletes that listener, its pool (and the
# pool's members) and its monitor — the primary listener stays.
resource "viettelidc_ovpc_load_balancer" "multi" {
  name              = "web-multi"
  subnet_id         = viettelidc_ovpc_subnet.public.id
  loadbalancer_type = "APPLICATION HTTP-HTTPS"
  package_type      = "LB Large"
  vpc_id            = data.viettelidc_ovpc_vpc.main.id

  # primary listener: plain HTTP
  listener_protocol = "HTTP"
  listener_port     = 80
  pool_members = [{ vm_id = viettelidc_ovpc_instance.vm1.id, port = 80, weight = 1 }]

  # added listener: HTTPS terminated at the LB with a certificate
  additional_listener {
    protocol       = "TERMINATED_HTTPS"
    port           = 443
    certificate_id = viettelidc_ovpc_certificate.web_cert.id

    pool_members {
      vm_id  = viettelidc_ovpc_instance.vm1.id
      port   = 443
      weight = 1
    }
  }
}
