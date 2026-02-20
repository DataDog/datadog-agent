// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers defines the ConfigProvider interface and includes
// implementations that collect check configurations from multiple sources (such
// as containers, files, etc.).
package providers

import (
	autoutils "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
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
		func(providerConfig *pkgconfigsetup.ConfigurationProviders, _ workloadmeta.Component, _ tagger.Component, _ workloadfilter.Component, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
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
	RegisterProvider(names.KubeServicesFileRegisterName, NewKubeServiceFileConfigProvider, providerCatalog)
	RegisterProvider(names.KubeServicesRegisterName, NewKubeServiceConfigProvider, providerCatalog)
	RegisterProviderWithComponents(names.PrometheusPodsRegisterName, NewPrometheusPodsConfigProvider, providerCatalog)
	RegisterProvider(names.ZookeeperRegisterName, NewZookeeperConfigProvider, providerCatalog)
	RegisterProviderWithComponents(names.ProcessLog, NewProcessLogConfigProvider, providerCatalog)

	prometheusServicesProvider := NewPrometheusServicesConfigProvider
	endpointsFileProvider := NewKubeEndpointsFileConfigProvider
	endpointsProvider := NewKubeEndpointsConfigProvider
	if autoutils.UseEndpointSlices() {
		endpointsFileProvider = NewKubeEndpointSlicesFileConfigProvider
		endpointsProvider = NewKubeEndpointSlicesConfigProvider
		prometheusServicesProvider = NewPrometheusServicesEndpointSlicesConfigProvider
	}
	RegisterProvider(names.PrometheusServicesRegisterName, prometheusServicesProvider, providerCatalog)
	RegisterProvider(names.KubeEndpointsFileRegisterName, endpointsFileProvider, providerCatalog)
	RegisterProvider(names.KubeEndpointsRegisterName, endpointsProvider, providerCatalog)
}
