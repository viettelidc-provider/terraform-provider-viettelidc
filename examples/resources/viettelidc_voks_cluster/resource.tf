# Example Usage - with cluster
resource "viettelidc_voks_cluster" "example" {
  name    = "k8s-cluster"
  version = "1.8.0"

  vpc_config {
    vpc_id = "0106"
  }
}


# Example Usage - with NFS
resource "viettelidc_voks_cluster" "example" {
  name    = "k8s-cluster"
  version = "1.8.0"

  vpc_config {
    vpc_id = "234134"
  }

  nfs = {
    additional_storage_size = 20
  }
}