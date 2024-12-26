# Example Usage
data "viettelidc_voks_cluster" "cluster" {

  id = "56231"

  #Attribute Reference
  name     = "k8s-cluster"
  status   = "SUCCESS"
  version  = "v1.29.8"
  endpoint = "https://172.17.11.53:6443"
  nfs = {
    cpu                = "2"
    memory             = "2"
    total_storage_size = "100"
    status             = "POWERED_ON"
    ip_address         = "10.20.29.230"
  }
  vpc_config = {
    security_group_ids = [11900, 11909, 11912]
    subnet_ids         = [4379, 4385]
    vpc_id             = 19490
  }
}