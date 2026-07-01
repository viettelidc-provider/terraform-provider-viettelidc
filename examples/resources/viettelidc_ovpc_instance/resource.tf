resource "viettelidc_ovpc_instance" "vm" {
  template_id        = 101
  subnet_id          = viettelidc_ovpc_subnet.private.id
  admin_pass         = "MySecretPass123!"
  cpu                = 2
  memory             = 4096
  storage_type       = "SSD"
  key_pair_name      = viettelidc_ovpc_key_pair.deploy.key_name
  security_group_ids = [viettelidc_ovpc_security_group.web.id]
  availability_zone  = "HN1"
  vpc_id             = viettelidc_ovpc_vpc.main.id
}
