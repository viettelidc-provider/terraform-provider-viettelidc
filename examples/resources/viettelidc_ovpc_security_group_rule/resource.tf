# Inbound SSH rule
resource "viettelidc_ovpc_security_group_rule" "ssh" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "SSH"
  source_ip         = "0.0.0.0/0"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}

# Custom inbound TCP rule
resource "viettelidc_ovpc_security_group_rule" "app" {
  security_group_id = viettelidc_ovpc_security_group.web.id
  direction         = "in"
  rule_type         = "Custom TCP"
  port              = "8080"
  source_ip         = "10.0.0.0/8"
  vpc_id            = viettelidc_ovpc_vpc.main.id
}
