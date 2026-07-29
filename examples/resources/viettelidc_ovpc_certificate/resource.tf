resource "viettelidc_ovpc_certificate" "tls" {
  name        = "my-tls-cert"
  vpc_id      = data.viettelidc_ovpc_vpc.main.id
  certificate = file("cert.pem")
  private_key = file("key.pem")
}
