terraform {
  required_providers {
    viettelidc = {
      source = "viettelidc/viettelidc"
    }
  }
}

resource "viettelidc_vdbs_security_group_rule" "allow_mysql" {
  security_group_id = "123"

  name          = "allow-mysql-inbound"
  type          = "MySQL"
  protocol_name = "TCP"
  port          = "3306"
  source        = "custom"
  source_ip     = "10.0.0.0/16"
}
