// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package networking implements the ViettelIDC IaC networking resources and
// data sources for the Plugin Framework provider:
//   - viettelidc_subnet (resource + 2 data sources)
//   - viettelidc_network_interface (resource + 2 data sources)
//   - viettelidc_network_interface_attachment (resource)
//   - viettelidc_route_table (resource + data source)
//   - viettelidc_route_table_association (resource)
//
// All resources call API via API Gateway. Request bodies use snake_case
// field names; the API Gateway renames them to the
// camelCase / vtt-prefixed names the API expects.
package networking

// Routed CSA endpoint paths. Centralised so a future move (e.g. version
// bump) is a single edit. Every endpoint accepts HTTP POST.
const (
	pathSubnetCreate = "/csa/api/v1/networking/subnet/create"
	pathSubnetDetail = "/csa/api/v1/networking/subnet/detail"
	pathSubnetUpdate = "/csa/api/v1/networking/subnet/update"
	pathSubnetDelete = "/csa/api/v1/networking/subnet/delete"
	pathSubnetList   = "/csa/api/v1/networking/subnet/list"

	pathVPCCreate = "/csa/api/v1/networking/virtual-private-cloud/create"
	pathVPCDetail = "/csa/api/v1/networking/virtual-private-cloud/detail"
	pathVPCUpdate = "/csa/api/v1/networking/virtual-private-cloud/update"
	pathVPCDelete = "/csa/api/v1/networking/virtual-private-cloud/delete"
	pathVPCList   = "/csa/api/v1/networking/virtual-private-cloud/list"

	// vFirewall (list-only)
	pathVFirewallList = "/csa/api/v1/vpc/firewall/list"

	pathNicCreate = "/csa/api/v1/networking/nic/create"
	pathNicDetail = "/csa/api/v1/networking/nic/detail"
	pathNicUpdate = "/csa/api/v1/networking/nic/update"
	pathNicDelete = "/csa/api/v1/networking/nic/delete"
	pathNicList   = "/csa/api/v1/networking/nic/private/list"
	pathNicAttach = "/csa/api/v1/networking/nic/attach/vm"
	pathNicDetach = "/csa/api/v1/networking/nic/detach/vm"

	pathFloatingIPCreate       = "/csa/api/v1/networking/floating-ip/create"
	pathFloatingIPDetail       = "/csa/api/v1/networking/floating-ip/detail"
	pathFloatingIPAssociate    = "/csa/api/v1/networking/floating-ip/associate"
	pathFloatingIPDisassociate = "/csa/api/v1/networking/floating-ip/disassociate"
	pathFloatingIPList         = "/csa/api/v1/networking/floating-ip/list"

	pathSGCreate = "/csa/api/v1/networking/security-group/create"
	pathSGDetail = "/csa/api/v1/networking/security-group/detail"
	pathSGUpdate = "/csa/api/v1/networking/security-group/update"
	pathSGDelete = "/csa/api/v1/networking/security-group/delete"
	pathSGList   = "/csa/api/v1/networking/security-group/list"

	pathSGRuleCreate       = "/csa/api/v1/networking/security-group/rule/create"
	pathSGRuleUpdate       = "/csa/api/v1/networking/security-group/rule/update"
	pathSGRuleInboundList  = "/csa/api/v1/networking/security-group/rule/inbound/list"
	pathSGRuleOutboundList = "/csa/api/v1/networking/security-group/rule/outbound/list"
	pathSGRuleTypes        = "/csa/api/v1/networking/security-group/rule/type"

	pathKeyPairCreate = "/csa/api/v1/vpc/keypair/create"
	pathKeyPairList   = "/csa/api/v1/vpc/keypair/paging/list"
	pathKeyPairDelete = "/csa/api/v1/vpc/keypair/delete"

	pathVMCreate = "/csa/api/v1/vm/create"
	pathVMDetail = "/csa/api/v1/vm/detail"
	pathVMList   = "/csa/api/v1/vpc/vm/list"
	pathVMStop   = "/csa/api/v1/vm/stop"
	pathVMUpdate = "/csa/api/v1/vm/update"
	pathVMDelete = "/csa/api/v1/vm/delete"

	pathVolumeCreate = "/csa/api/v1/storage/volume/create"
	pathVolumeDetail = "/csa/api/v1/storage/volume/detail"
	pathVolumeUpdate = "/csa/api/v1/storage/volume/update"
	pathVolumeDelete = "/csa/api/v1/storage/volume/delete"
	pathVolumeList   = "/csa/api/v1/storage/volume/list"

	pathVolumeAttach = "/csa/api/v1/storage/volume/attach/vm"
	pathVolumeDetach = "/csa/api/v1/storage/volume/detach/vm"

	pathRouteTableCreate       = "/csa/api/v1/networking/route-table/create"
	pathRouteTableDetail       = "/csa/api/v1/networking/route-table/detail"
	pathRouteTableDelete       = "/csa/api/v1/networking/route-table/delete"
	pathRouteTableList         = "/csa/api/v1/networking/route-table/list"
	pathRouteTableSubnetAttach = "/csa/api/v1/networking/route-table/subnet/attach"
	pathRouteTableSubnetDetach = "/csa/api/v1/networking/route-table/subnet/detach"
	pathRouteTableAssociation  = "/csa/api/v1/networking/route-table/association/subnet/all"
	pathRouteTableAvailable    = "/csa/api/v1/networking/route-table/available/subnet/all"

	pathNatGatewayCreate = "/csa/api/v1/nat-gateway/create"
	pathNatGatewayDelete = "/csa/api/v1/nat-gateway/delete"
	pathNatGatewayList   = "/csa/api/v1/nat-gateway/list"

	pathSubnetListAvailableForNat = "/csa/api/v1/networking/subnet/list-available-isolated-for-nat"

	pathInternetGatewayList          = "/csa/api/v1/networking/internet-gateway/list"
	pathInternetGatewayListForAssign = "/csa/api/v1/networking/internet-gateway/list-for-assign"

	pathLoadBalancerCreate      = "/csa/api/v1/networking/loadbalancer/compound-create"
	pathLoadBalancerDelete      = "/csa/api/v1/networking/loadbalancer/delete"
	pathLoadBalancerUpdate      = "/csa/api/v1/networking/loadbalancer/update"
	pathLoadBalancerDetail      = "/csa/api/v1/networking/loadbalancer/detail"
	pathLoadBalancerList        = "/csa/api/v1/networking/loadbalancer/list"
	pathLoadBalancerListeners   = "/csa/api/v1/networking/loadbalancer/listener-by-lb/all"
	pathLoadBalancerPools       = "/csa/api/v1/networking/loadbalancer/pool-by-lb/all"
	pathLoadBalancerListLayer7  = "/csa/api/v1/networking/loadbalancer/list-all-layer-7"
	pathLoadBalancerListTypes   = "/csa/api/v1/networking/loadbalancer/list-loadbalancer-type"
	pathLoadBalancerAttachedNic = "/csa/api/v1/networking/loadbalancer/attached-nic/list"

	// Certificate (key-manager service — RESTful, vpcId in URL path)
	pathCertBase = "/key-manager/api/v1/kms" // append /{vpcId}/certificate[/{certId}]

	pathBackupPlanCreate = "/csa/api/v1/storage/backup-plan/create"
	pathBackupPlanDelete = "/csa/api/v1/storage/backup-plan/delete"
	pathBackupPlanUpdate = "/csa/api/v1/storage/backup-plan/update"
	pathBackupPlanList   = "/csa/api/v1/storage/backup-plan/list"

	pathBackupRecordList = "/csa/api/v1/storage/backup/record/list"

	pathRegionHostsByCust    = "/csa/api/v1/region/list-host-by-customer"
	pathRegionHostsByOrder   = "/csa/api/v1/region/list-host-by-order"
	pathVMTemplateList = "/csa/api/v1/host-information/list-template"
)

// listWarningThreshold triggers a Diagnostics warning on list-style data
// sources when the result count meets/exceeds this value (likely truncated
// by CSA's default page size).
const listWarningThreshold = 1000
