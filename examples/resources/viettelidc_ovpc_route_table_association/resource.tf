resource "viettelidc_ovpc_route_table_association" "assoc" {
  route_table_id = viettelidc_ovpc_route_table.main.id
  subnet_id      = viettelidc_ovpc_subnet.private.id
  vpc_id         = viettelidc_ovpc_vpc.main.id
}
