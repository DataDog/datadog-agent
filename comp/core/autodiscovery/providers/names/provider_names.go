// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package names defines the name of each config provider ("container",
// "cluster-checks", "file", etc.).
package names

// User-facing names for the config providers
const (
	Consul                           = "consul"
	Container                        = "container"
	CloudFoundryBBS                  = "cloudfoundry-bbs"
	ClusterChecks                    = "cluster-checks"
	EndpointsChecks                  = "endpoints-checks"
	Etcd                             = "etcd"
	File                             = "file"
	KubeContainer                    = "kubernetes-container-allinone"
	Kubernetes                       = "kubernetes"
	KubeServices                     = "kubernetes-services"
	KubeServicesFile                 = "kubernetes-services-file"
	KubeEndpoints                    = "kubernetes-endpoints"
	KubeEndpointSlices               = "kubernetes-endpointslices"
	KubeEndpointsFile                = "kubernetes-endpoints-file"
	KubeEndpointSlicesFile           = "kubernetes-endpointslices-file"
	KubeCRD                          = "kubernetes-crd"
	ProcessLog                       = "process_log"
	PrometheusPods                   = "prometheus-pods"
	PrometheusServices               = "prometheus-services"
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
