// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vks

// VKS API paths for Kubernetes Service (Phase 6).
// All operations route through CSA backend.

const (
	pathClusterDetail        = "/csa/api/v1/kubernetes/cluster/detail"
	pathClusterDelete        = "/csa/api/v1/kubernetes/cluster/group/delete"
	pathClusterVersionUpdate = "/csa/api/v1/kubernetes/cluster/version/update"
	pathClusterDetailNetwork = "/csa/api/v1/kubernetes/cluster/detail-network"

	// Cluster Autoscale Config
	pathClusterAutoscaleConfigDetail = "/csa/api/v1/kubernetes/cluster/config-autoscale/detail"
	pathClusterAutoscaleConfigUpdate = "/csa/api/v1/kubernetes/cluster/config-autoscale/update"

	pathNodeGroupCreate = "/csa/api/v1/kubernetes/cluster/node-group/create"
	pathNodeGroupDetail = "/csa/api/v1/kubernetes/cluster/group/detail"
	pathNodeGroupEdit   = "/csa/api/v1/kubernetes/cluster/group/edit"
	pathNodeGroupDelete = "/csa/api/v1/kubernetes/cluster/group/delete"

	// Non-IaC details
	pathClusterNodesList         = "/csa/api/v1/kubernetes/cluster/node/list"
	pathClusterSubnetsList       = "/csa/api/v1/kubernetes/cluster/networking/subnets/list"
	pathClusterSubnetsDetail     = "/csa/api/v1/kubernetes/cluster/networking/subnets/detail"
	pathClusterNICsList          = "/csa/api/v1/kubernetes/cluster/networking/network-interface/list"
	pathClusterNICsDetail        = "/csa/api/v1/kubernetes/cluster/networking/network-interface/detail"
	pathClusterSGsList           = "/csa/api/v1/kubernetes/cluster/networking/security-group/list"
	pathClusterSGsDetail         = "/csa/api/v1/kubernetes/cluster/networking/security-group/detail"
	pathNFSServerDetail          = "/csa/api/v1/kubernetes/cluster/detailNfsServer"
	pathAutoscaleHistory         = "/csa/api/v1/kubernetes/cluster/auto-scale/history"
	pathClusterEventsList        = "/csa/api/v1/kubernetes/cluster/event/list"
	pathClusterEventsExport      = "/csa/api/v1/kubernetes/cluster/event/export"
	pathClusterListPaging        = "/csa/api/v1/kubernetes/cluster/list-paging"
	pathClusterList              = "/csa/api/v1/kubernetes/cluster/list"
	pathClusterUpgradeCheck      = "/csa/api/v1/kubernetes/cluster/check-upgrade"
	pathUpgradeVersionDetail     = "/csa/api/v1/kubernetes/cluster/upgrade-version/detail"
	pathCloudwatchAddonsCheck    = "/csa/api/v1/kubernetes/cluster/check-addons-cloudwatch"
	pathAddonAccessInfo          = "/csa/api/v1/kubernetes/cluster/addon/access-info"
	pathClusterNodeDetail        = "/csa/api/v1/kubernetes/cluster/detailClusterNode"
	pathNodegroupLabelsList      = "/csa/api/v1/kubernetes/cluster/node-group/label/list"
	pathNodegroupTaintsList      = "/csa/api/v1/kubernetes/cluster/node-group/taint/list"
	pathNodegroupTemplatesList   = "/csa/api/v1/kubernetes/cluster/group/template/list"
	pathClusterNFSAddons         = "/csa/api/v1/kubernetes/cluster/nfs/add-ons"
	pathClusterNFSDetail         = "/csa/api/v1/kubernetes/cluster/nfs/detail"

	// Platform general APIs
	pathProviderList         = "/csa/api/v1/provider/list"
	pathRegionHostsByCust    = "/csa/api/v1/region/list-host-by-customer"
	pathCustomerInfo         = "/csa/api/v1/customer/info"
	pathCustomerCaptcha      = "/csa/api/v1/customer/captcha"
	pathCustomerSupportInfo  = "/csa/api/v1/customer/support-info"

	// Standard Addon / Kubeconfig Endpoints
	pathAddonInstall       = "/csa/api/v1/kubernetes/cluster/addon/install"
	pathAddonUninstall     = "/csa/api/v1/kubernetes/cluster/addon/uninstall"
	pathAddonList          = "/csa/api/v1/kubernetes/cluster/addon/list"
	pathKubeconfigDownload = "/csa/api/v1/kubernetes/cluster/download-config"

	// Scheduler Endpoints
	pathK8sSchedulerCreateEdit = "/csa/api/v1/kubernetes/cluster/block-storage/create-edit-schedule"
	pathK8sSchedulerDelete     = "/csa/api/v1/kubernetes/cluster/block-storage/delete-schedule"
	pathK8sSchedulerListPaging = "/csa/api/v1/kubernetes/cluster/block-storage/schedule/list-paging"
	pathK8sSchedulerListAll    = "/csa/api/v1/kubernetes/cluster/block-storage/schedule/list-all"
	pathK8sSchedulerBackupList = "/csa/api/v1/kubernetes/cluster/block-storage/backup/list-paging"

	// Manual Backup Endpoints
	pathK8sBackupManualCreate  = "/csa/api/v1/kubernetes/cluster/block-storage/create-backup-manual"
	pathK8sBackupManualDelete  = "/csa/api/v1/kubernetes/cluster/block-storage/delete-backup-manual"
	pathK8sBlockStorageList    = "/csa/api/v1/kubernetes/cluster/block-storage/list"
)
