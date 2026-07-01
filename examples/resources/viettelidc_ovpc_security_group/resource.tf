resource "viettelidc_ovpc_security_group" "web" {
  name        = "web-sg"
  description = "Security group for web servers"
  vpc_id      = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_security_group_rule" "allow_http" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "HTTP"
  source_ip         = "0.0.0.0/0"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}
