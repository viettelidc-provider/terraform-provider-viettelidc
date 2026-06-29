# Táº¡o Parameter Group cho MySQL 8.0 vÃ  gáº¯n vÃ o instance
resource "viettelidc_vdbs_parameter_group" "custom_pg" {
  name        = "uc4-mysql8-parameter-group-20"
  family      = "mysql8.0"
  description = "Parameter group managed by Terraform for UC4"
  instance_id = "2851"

  parameter = [
    {
      name  = "max_connections"
      value = "150"
    },
    {
      name  = "innodb_buffer_pool_size"
      value = "268435456" # 256MB
    }
  ]
}
