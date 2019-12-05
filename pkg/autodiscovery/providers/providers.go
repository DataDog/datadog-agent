// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// KubeEndpointsProviderName defines the kube endpoints provider name
const KubeEndpointsProviderName = "kube_endpoints"

// ProviderCatalog keeps track of config providers by name
var ProviderCatalog = make(map[string]ConfigProviderFactory)

// RegisterProvider adds a loader to the providers catalog
func RegisterProvider(name string, factory ConfigProviderFactory) {
	ProviderCatalog[name] = factory
}

// ConfigProviderFactory is any function capable to create a ConfigProvider instance
type ConfigProviderFactory func(cfg config.ConfigurationProviders) (ConfigProvider, error)

// ProviderCache contains the number of AD Templates and the latest Index
type ProviderCache struct {
	LatestTemplateIdx float64
	NumAdTemplates    int
}

// NewCPCache instantiate a ProviderCache.
func NewCPCache() *ProviderCache {
	return &ProviderCache{
		LatestTemplateIdx: 0,
		NumAdTemplates:    0,
	}
}

// ConfigProvider is the interface that wraps the Collect method
//
// Collect is responsible of populating a list of CheckConfig instances
// by retrieving configuration patterns from external resources: files
// on disk, databases, environment variables are just few examples.
//
// Any type implementing the interface will take care of any dependency
// or data needed to access the resource providing the configuration.
// IsUpToDate checks the local cache of the CP and returns accordingly.
type ConfigProvider interface {
	Collect() ([]integration.Config, error)
	String() string
	IsUpToDate() (bool, error)
}
