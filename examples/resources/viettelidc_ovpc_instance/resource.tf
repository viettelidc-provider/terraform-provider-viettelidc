data "viettelidc_ovpc_vm_templates" "ubuntu" {
  name_filter = "Ubuntu 22.04"
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
}

resource "viettelidc_ovpc_instance" "vm" {
  name               = "web-server"
  template_id        = tonumber(data.viettelidc_ovpc_vm_templates.ubuntu.templates[0].id)
  subnet_id          = viettelidc_ovpc_subnet.private.id
  admin_pass         = "MySecretPass123!"
  cpu                = 2
  memory             = 4 # GB
  storage            = 40 # GiB
  storage_type       = "SSD"
  key_pair_name      = viettelidc_ovpc_key_pair.deploy.key_name
  security_group_ids = [viettelidc_ovpc_security_group.web.id]
  vpc_id             = data.viettelidc_ovpc_vpc.main.id
}
