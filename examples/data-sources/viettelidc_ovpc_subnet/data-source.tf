data "viettelidc_ovpc_subnet" "private" {
  name   = "private-subnet"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
