data "viettelidc_ovpc_security_group" "default" {
  name   = "default-sg"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
