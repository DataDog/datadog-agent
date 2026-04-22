// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package listeners is a wrapper that registers the available autodiscovery listerners.
package listeners

import autoutils "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"

const (
	cloudFoundryBBSListenerName = "cloudfoundry-bbs"
	containerListenerName       = "container"
	environmentListenerName     = "environment"
	kubeEndpointsListenerName   = "kube_endpoints"
	kubeServicesListenerName    = "kube_services"
	kubeletListenerName         = "kubelet"
	processListenerName         = "process"
	snmpListenerName            = "snmp"
	staticConfigListenerName    = "static config"
	dbmAuroraListenerName       = "database-monitoring-aurora"
	dbmRdsListenerName          = "database-monitoring-rds"
	crdListenerName             = "kube_crd"
)

// RegisterListeners registers the available autodiscovery listerners.
func RegisterListeners(serviceListenerFactories map[string]ServiceListenerFactory) {
	// register the available listeners
	Register(cloudFoundryBBSListenerName, NewCloudFoundryListener, serviceListenerFactories)
	Register(containerListenerName, NewContainerListener, serviceListenerFactories)
	Register(environmentListenerName, NewEnvironmentListener, serviceListenerFactories)
	Register(kubeServicesListenerName, NewKubeServiceListener, serviceListenerFactories)
	Register(kubeletListenerName, NewKubeletListener, serviceListenerFactories)
	Register(processListenerName, NewProcessListener, serviceListenerFactories)
	Register(snmpListenerName, NewSNMPListener, serviceListenerFactories)
	Register(staticConfigListenerName, NewStaticConfigListener, serviceListenerFactories)
	Register(dbmAuroraListenerName, NewDBMAuroraListener, serviceListenerFactories)
	Register(dbmRdsListenerName, NewDBMRdsListener, serviceListenerFactories)
	Register(crdListenerName, NewCRDListerner, serviceListenerFactories)

	endpointsListener := NewKubeEndpointsListener
	if autoutils.UseEndpointSlices() {
		endpointsListener = NewKubeEndpointSlicesListener
	}
	Register(kubeEndpointsListenerName, endpointsListener, serviceListenerFactories)
}
