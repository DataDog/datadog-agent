// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package listeners is a wrapper that registers the available autodiscovery listerners.
package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	cloudFoundryBBSListenerName = "cloudfoundry_bbs"
	containerListenerName       = "container"
	environmentListenerName     = "environment"
	kubeEndpointsListenerName   = "kube_endpoints"
	kubeServicesListenerName    = "kube_services"
	kubeletListenerName         = "kubelet"
	snmpListenerName            = "snmp"
	staticConfigListenerName    = "static config"
	dbmAuroraListenerName       = "database-monitoring-aurora"
)

// RegisterListeners registers the available autodiscovery listerners.
func RegisterListeners(serviceListenerFactories map[string]ServiceListenerFactory, wmeta optional.Option[workloadmeta.Component]) {
	// register the available listeners
	Register(cloudFoundryBBSListenerName, NewCloudFoundryListener, serviceListenerFactories)
	Register(containerListenerName, func(config Config, telemetryStore *telemetry.Store) (ServiceListener, error) {
		return NewContainerListener(config, wmeta, telemetryStore)
	}, serviceListenerFactories)
	Register(environmentListenerName, NewEnvironmentListener, serviceListenerFactories)
	Register(kubeEndpointsListenerName, NewKubeEndpointsListener, serviceListenerFactories)
	Register(kubeServicesListenerName, NewKubeServiceListener, serviceListenerFactories)
	Register(kubeletListenerName, func(config Config, telemetryStore *telemetry.Store) (ServiceListener, error) {
		return NewKubeletListener(config, wmeta, telemetryStore)
	}, serviceListenerFactories)
	Register(snmpListenerName, NewSNMPListener, serviceListenerFactories)
	Register(staticConfigListenerName, NewStaticConfigListener, serviceListenerFactories)
	Register(dbmAuroraListenerName, NewDBMAuroraListener, serviceListenerFactories)
}
