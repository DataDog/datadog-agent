// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package names

// User-facing names for the config providers
const (
	Consul             = "consul"
	Container          = "container"
	CloudFoundryBBS    = "cloudfoundry-bbs"
	ClusterChecks      = "cluster-checks"
	EndpointsChecks    = "endpoints-checks"
	Etcd               = "etcd"
	File               = "file"
	KubeContainer      = "kubernetes-container-allinone"
	Kubernetes         = "kubernetes"
	KubeServices       = "kubernetes-services"
	KubeServicesFile   = "kubernetes-services-file"
	KubeEndpoints      = "kubernetes-endpoints"
	KubeEndpointsFile  = "kubernetes-endpoints-file"
	PrometheusPods     = "prometheus-pods"
	PrometheusServices = "prometheus-services"
	SNMP               = "snmp"
	Zookeeper          = "zookeeper"
)

// Internal Autodiscovery names for the config providers
// Some of these names are different from the user-facing names
// And they're kept unchanged for backward compatibility
// as they could be hardcoded in the agent config.
const (
	ConsulRegisterName             = "consul"
	ClusterChecksRegisterName      = "clusterchecks"
	EndpointsChecksRegisterName    = "endpointschecks"
	EtcdRegisterName               = "etcd"
	KubeletRegisterName            = "kubelet"
	KubeContainerRegisterName      = "kubernetes-container-allinone"
	KubeServicesRegisterName       = "kube_services"
	KubeServicesFileRegisterName   = "kube_services_file"
	KubeEndpointsRegisterName      = "kube_endpoints"
	KubeEndpointsFileRegisterName  = "kube_endpoints_file"
	PrometheusPodsRegisterName     = "prometheus_pods"
	PrometheusServicesRegisterName = "prometheus_services"
	ZookeeperRegisterName          = "zookeeper"
)
