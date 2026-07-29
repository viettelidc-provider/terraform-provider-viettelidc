# The VPC is created by the platform - look it up, do not declare it.
data "viettelidc_ovpc_vpc" "main" {
  name = "my-vpc"
}

resource "viettelidc_ovpc_subnet" "private" {
  name            = "private-subnet"
  network_address = "10.0.1.0/24"
  is_public_zone  = false
  vpc_id          = data.viettelidc_ovpc_vpc.main.id
  description     = "Private subnet"
}

resource "viettelidc_ovpc_subnet" "public" {
  name            = "public-subnet"
  network_address = "10.0.2.0/24"
  is_public_zone  = true
  vpc_id          = data.viettelidc_ovpc_vpc.main.id
}
