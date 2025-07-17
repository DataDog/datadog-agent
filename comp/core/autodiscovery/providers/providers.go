// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers defines the ConfigProvider interface and includes
// implementations that collect check configurations from multiple sources (such
// as containers, files, etc.).
package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RegisterProvider adds a loader to the providers catalog
func RegisterProvider(name string,
	factory func(providerConfig *pkgconfigsetup.ConfigurationProviders, telemetryStore *telemetry.Store) (types.ConfigProvider, error),
	providerCatalog map[string]types.ConfigProviderFactory) {
	RegisterProviderWithComponents(
		name,
		func(providerConfig *pkgconfigsetup.ConfigurationProviders, _ workloadmeta.Component, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
			return factory(providerConfig, telemetryStore)
		},
		providerCatalog,
	)
}

// RegisterProviderWithComponents adds a loader to the providers catalog
func RegisterProviderWithComponents(name string, factory types.ConfigProviderFactory, providerCatalog map[string]types.ConfigProviderFactory) {
	if factory == nil {
		log.Infof("ConfigProvider factory %s does not exist.", name)
		return
	}
	_, registered := providerCatalog[name]
	if registered {
		log.Errorf("ConfigProvider factory %s already registered. Ignoring.", name)
		return
	}
	providerCatalog[name] = factory
}

// RegisterProviders adds all the default providers to the catalog
func RegisterProviders(providerCatalog map[string]types.ConfigProviderFactory) {
	RegisterProvider(names.CloudFoundryBBS, NewCloudFoundryConfigProvider, providerCatalog)
	RegisterProvider(names.ClusterChecksRegisterName, NewClusterChecksConfigProvider, providerCatalog)
	RegisterProvider(names.ConsulRegisterName, NewConsulConfigProvider, providerCatalog)
	RegisterProviderWithComponents(names.KubeContainer, NewContainerConfigProvider, providerCatalog)
	RegisterProvider(names.EndpointsChecksRegisterName, NewEndpointsChecksConfigProvider, providerCatalog)
	RegisterProvider(names.EtcdRegisterName, NewEtcdConfigProvider, providerCatalog)
	RegisterProvider(names.KubeEndpointsFileRegisterName, NewKubeEndpointsFileConfigProvider, providerCatalog)
	RegisterProvider(names.KubeEndpointsRegisterName, NewKubeEndpointsConfigProvider, providerCatalog)
	RegisterProvider(names.KubeServicesFileRegisterName, NewKubeServiceFileConfigProvider, providerCatalog)
	RegisterProvider(names.KubeServicesRegisterName, NewKubeServiceConfigProvider, providerCatalog)
	RegisterProvider(names.PrometheusPodsRegisterName, NewPrometheusPodsConfigProvider, providerCatalog)
	RegisterProvider(names.PrometheusServicesRegisterName, NewPrometheusServicesConfigProvider, providerCatalog)
	RegisterProvider(names.ZookeeperRegisterName, NewZookeeperConfigProvider, providerCatalog)
	RegisterProviderWithComponents(names.GPU, NewGPUConfigProvider, providerCatalog)
}
