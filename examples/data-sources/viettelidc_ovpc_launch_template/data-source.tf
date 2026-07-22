data "viettelidc_ovpc_launch_template" "web" {
  name   = "web-template"
  vpc_id = data.viettelidc_ovpc_vpc.main.id
}
