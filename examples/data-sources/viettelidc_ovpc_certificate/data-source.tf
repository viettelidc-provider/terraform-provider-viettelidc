data "viettelidc_ovpc_certificate" "tls" {
  name   = "my-tls-cert"
  vpc_id = viettelidc_ovpc_vpc.main.id
}
