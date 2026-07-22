# Configuration-based authentication.
#
# Never commit a password. Read it from a variable or the environment:
# VIETTELIDC_EMAIL / VIETTELIDC_PASSWORD / VIETTELIDC_MFA_CODE.
variable "viettelidc_password" {
  type      = string
  sensitive = true
}

provider "viettelidc" {
  domain_id = "3b3e6994-4b04-40ea-bedc-5befd874d73a"
  username  = "iac"
  password  = var.viettelidc_password
  mfa_code  = var.mfa_code
}
