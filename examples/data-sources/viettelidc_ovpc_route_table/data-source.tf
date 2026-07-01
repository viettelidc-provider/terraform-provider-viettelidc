data "viettelidc_ovpc_route_table" "main" {
  name   = "main-rt"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
