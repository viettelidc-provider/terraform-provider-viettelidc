data "viettelidc_ovpc_security_group" "default" {
  name   = "default-sg"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
