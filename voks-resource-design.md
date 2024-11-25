# vOKS IaC Resource Design

## Cluster

1. **Resources**

```terraform
resource "viettelidc_voks_cluster" "cluster" {
  name = "k8s-cluster"
  version = "1.8.0"
  vpc_config {
    id = "234134"
  }
  nfs = {
    additional_storage_size = 20
  }
  version = "1.8.0"
}
```

2. **Datasources**

```terraform
data "viettelidc_voks_cluster" "cluster" {
  
  id = "56231"
  name = "k8s-cluster"
  status = "SUCCESS" 
  version = "1.8.0"
  endpoint = "https://203.0.113.10:6443"
  
  nfs = {
    cpu = 4
    ram = 4
    total_storage_size = 240
    additional_storage_size = 20 //Optional
    status = power_on
    ip_address = 10.13.78.238
  }

  vpc_config {
    vpc_id = "234134"
    security_group_ids = ["3153", "2718"]
    subnet_ids = ["7281", "9182"]
  }
  
  created_at = "2024-01-01 00:00:00"
  updated_at = "2024-01-01 00:00:00"
}
```

## Node Group
1. **Resources**

```terraform
resource "viettelidc_voks_node_group" "node-group" {
  
  cluster_name = viettelidc_voks_cluster.cluster.name
  name = "k8s-node-group"
  instance_type = "T1.vOKS 1"
  nfs_size = 20

  scaling_config {
    enable_auto_scale = true
    max_size     = 2
    min_size     = 1
  }

  labels = {
    environment = "production"
    team        = "devops"
    app         = "backend"
    region      = "us-west"
  }

  taint {
    key    = "dedicated"
    value  = "gpu"
    effect = "NO_SCHEDULE"
  }
  
  taint {
    key    = "a"
    value  = "gpu"
    effect = "NO_SCHEDULE"
  }
  
  taint {
    key    = "b"
    value  = "gpu"
    effect = "NO_SCHEDULE"
  }
}
```

2. **Data sources**

```terraform
resource "viettelidc_voks_node_group" "node-group" {
  
  id = "1231"
  cluster_name = "k8s-cluster"
  name = "k8s-node-group"
  instance_type = "T1.vOKS 1"
  nfs_size = 20
  nic_ids = ["3213", "8172"]
  status = "SUCCESS"
  
  nodes = [{
    node_name = "worker-7bscd"
    type = "WORKER"
    instance_type = "2 vCPU - 4GB RAM - 50GB SSD"
    start_time = "2024-01-01 00:00:00"
    end_time = "2024-01-01 23:00:00"
  }]

  scaling_config {
    enable_auto_scale = true
    max_size     = 2
    min_size     = 1
  }

  labels = {
    environment = "production"
    team        = "devops"
    app         = "backend"
    region      = "us-west"
  }

  taint {
    key    = "dedicated"
    value  = "gpu"
    effect = "NO_SCHEDULE"
  }
}
```

## Add-on

1. **Resources**

```terraform
resource "viettelidc_voks_addon" "addon" {
  cluster_name                = viettelidc_voks_cluster.cluster.name
  addon_name                  = "coredns"
  addon_version               = "v1.10.1-eksbuild.1"
  resolve_conflicts_on_update = "PRESERVE"
}
```

2. **Data sources**

```terraform
resource "viettelidc_voks_addon" "addon" {
  cluster_name                = viettelidc_voks_cluster.cluster.name
  addon_name                  = "coredns"
  addon_version               = "v1.10.1-eksbuild.1"
  resolve_conflicts_on_update = "PRESERVE"
  status = "SUCCESS"
}
```