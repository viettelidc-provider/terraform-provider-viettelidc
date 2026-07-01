data "viettelidc_ovpc_internet_gateway" "igw" {
  name   = "default-igw"
  vpc_id = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_nat_gateway" "nat" {
  name                = "main-nat"
  subnet_id           = viettelidc_ovpc_subnet.public.id
  internet_gateway_id = data.viettelidc_ovpc_internet_gateway.igw.id
  connect_type        = false
  vpc_id              = viettelidc_ovpc_vpc.main.id
}
