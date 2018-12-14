// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// User-facing names for the config providers
const (
	Consul        = "consul"
	ClusterChecks = "cluster-checks"
	Docker        = "docker"
	ECS           = "ecs"
	Etcd          = "etcd"
	File          = "file"
	Kubernetes    = "kubernetes"
	KubeServices  = "kubernetes-services"
	Zookeeper     = "zookeeper"
)

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
