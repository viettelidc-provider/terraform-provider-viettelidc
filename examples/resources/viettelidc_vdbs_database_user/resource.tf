# Táº¡o Database User cho Instance
resource "viettelidc_vdbs_database_user" "app_user" {
  name        = "appuser12"
  password    = "Vtdc-123456"
  instance_id = "2851"
  host        = "%"
  schemas     = ["quannm"]
}
