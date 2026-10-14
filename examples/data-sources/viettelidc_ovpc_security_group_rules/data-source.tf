data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

data "viettelidc_ovpc_security_group" "app" {
  name   = "app-sg"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

data "viettelidc_ovpc_security_group_rules" "inbound" {
  security_group_id = data.viettelidc_ovpc_security_group.app.id
  direction         = "in"
  vpc_id            = data.viettelidc_ovpc_vpc.main.id
}

output "inbound_rules" {
  value = data.viettelidc_ovpc_security_group_rules.inbound.rules
}
