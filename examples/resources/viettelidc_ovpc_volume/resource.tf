resource "viettelidc_ovpc_volume" "data" {
  name        = "data-disk"
  size        = 100
  volume_type = "SSD"
  vpc_id      = viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_volume_attachment" "attach" {
  instance_id = viettelidc_ovpc_instance.vm.id
  volume_id   = viettelidc_ovpc_volume.data.id
  vpc_id      = viettelidc_ovpc_vpc.main.id
}
