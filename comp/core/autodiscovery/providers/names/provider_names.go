// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package names defines the name of each config provider ("container",
// "cluster-checks", "file", etc.).
package names

// User-facing names for the config providers
const (
	// Consul discovers check configurations stored in a Consul KV store.
	Consul = "consul"
	// Container discovers checks from running containers via Docker labels or pod annotations.
	Container = "container"
	// CloudFoundryBBS discovers checks from Cloud Foundry apps via the BBS API.
	CloudFoundryBBS = "cloudfoundry-bbs"
	// ClusterChecks distributes check configurations across nodes via the cluster-agent.
	ClusterChecks = "cluster-checks"
	// EndpointsChecks discovers checks for Kubernetes service endpoints via the cluster-agent.
	EndpointsChecks = "endpoints-checks"
	// Etcd discovers check configurations stored in an etcd KV store.
	Etcd = "etcd"
	// File loads check configurations from YAML files in the conf.d directory.
	File = "file"
	// KubeContainer is an all-in-one provider that handles both pod and container annotations.
	KubeContainer = "kubernetes-container-allinone"
	// Kubernetes discovers checks from Kubernetes pod annotations.
	Kubernetes = "kubernetes"
	// KubeServices discovers checks from Kubernetes service annotations.
	KubeServices = "kubernetes-services"
	// KubeServicesFile loads Kubernetes service check configurations from YAML files.
	KubeServicesFile = "kubernetes-services-file"
	// KubeEndpoints discovers checks from Kubernetes endpoint from service annotations.
	KubeEndpoints = "kubernetes-endpoints"
	// KubeEndpointSlices discovers checks from Kubernetes EndpointSlice from service annotations.
	KubeEndpointSlices = "kubernetes-endpointslices"
	// KubeEndpointsFile loads Kubernetes endpoint check configurations from YAML files.
	KubeEndpointsFile = "kubernetes-endpoints-file"
	// KubeEndpointSlicesFile loads Kubernetes EndpointSlice check configurations from YAML files.
	KubeEndpointSlicesFile = "kubernetes-endpointslices-file"
	// KubeCRD discovers check configurations from YAML files that target Kubernetes CRDs via advanced AD identifiers.
	KubeCRD = "kubernetes-crd"
	// ProcessLog autodiscovers log collection configurations from running processes.
	ProcessLog = "process_log"
	// PrometheusPods discovers Prometheus scrape targets from Kubernetes pod annotations.
	PrometheusPods = "prometheus-pods"
	// PrometheusServices discovers Prometheus scrape targets from Kubernetes service annotations.
	PrometheusServices = "prometheus-services"
	// PrometheusServicesEndpointSlices discovers Prometheus targets from EndpointSlice-backed services.
	PrometheusServicesEndpointSlices = "prometheus-services-endpointslices"
	// RemoteConfig delivers check configurations pushed from the Datadog backend via Remote Configuration.
	RemoteConfig = "remote-config"
	// SNMP autodiscovers SNMP devices on configured subnets.
	SNMP = "snmp"
	// Zookeeper discovers check configurations stored in a Zookeeper ZNode tree.
	Zookeeper = "zookeeper"
	// GPU discovers GPU devices and generates check configurations for GPU monitoring.
	GPU = "gpu"
	// DataStreamsLiveMessages provides live message sampling configurations for Data Streams Monitoring.
	DataStreamsLiveMessages = "dsm-live-messages"
	// DOQueryActions provides check configurations for Database Observability query-level actions.
	DOQueryActions = "do-query-actions"
	// PrometheusHTTPSD discovers check configurations from a Prometheus HTTP Service Discovery endpoint.
	PrometheusHTTPSD = "prometheus-http-sd"
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
	KubeCrdRegisterName            = "kube_crd"
	PrometheusPodsRegisterName     = "prometheus_pods"
	PrometheusServicesRegisterName = "prometheus_services"
	PrometheusHTTPSDRegisterName   = "prometheus_http_sd"
	RemoteConfigRegisterName       = "remote_config"
	ZookeeperRegisterName          = "zookeeper"
)
