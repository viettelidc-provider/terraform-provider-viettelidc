# Look up by id
data "viettelidc_ovpc_volume" "by_id" {
  id     = "228025"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}

# Look up by name (exact match)
data "viettelidc_ovpc_volume" "by_name" {
  name   = "root-vm-2c9b96bd"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
