data "viettelidc_ovpc_internet_gateway" "igw" {
  name   = "default-igw"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_nat_gateway" "nat" {
  name = "main-nat"
  # A NAT Gateway attaches to a private subnet (is_public_zone = false) — it is
  # what gives that subnet outbound internet. A public subnet is rejected.
  subnet_id           = viettelidc_ovpc_subnet.private.id
  internet_gateway_id = data.viettelidc_ovpc_internet_gateway.igw.id
  connect_type        = false
  vpc_id              = data.viettelidc_ovpc_vpc.main.id
}
