data "viettelidc_ovpc_nat_gateway" "nat" {
  name   = "main-nat"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
