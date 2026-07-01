resource "viettelidc_ovpc_vpc" "main" {
  name       = "my-vpc"
  cidr_block = "10.0.0.0/16"
}

resource "viettelidc_ovpc_subnet" "private" {
  name            = "private-subnet"
  network_address = "10.0.1.0/24"
  is_public_zone  = false
  vpc_id          = viettelidc_ovpc_vpc.main.id
  description     = "Private subnet"
}

resource "viettelidc_ovpc_subnet" "public" {
  name            = "public-subnet"
  network_address = "10.0.2.0/24"
  is_public_zone  = true
  vpc_id          = viettelidc_ovpc_vpc.main.id
}
