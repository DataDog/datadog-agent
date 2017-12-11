// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// ProviderCatalog keeps track of config providers by name
var ProviderCatalog = make(map[string]ConfigProviderFactory)

// RegisterProvider adds a loader to the providers catalog
func RegisterProvider(name string, factory ConfigProviderFactory) {
	ProviderCatalog[name] = factory
}

// ConfigProviderFactory is any function capable to create a ConfigProvider instance
type ConfigProviderFactory func(cfg config.ConfigurationProviders) (ConfigProvider, error)

// Cache Provider.
type CacheProvider struct {
	Adids2Node map[string]AdIdentfier2stats // ["foo": Stat] Only 1 ad_identifier per tuple Stat
}
type AdIdentfier2stats struct {
	Stats map[string]int32	// Stat = ["check_names":1,"init_configs":0,"instances":0]
}
func NewCPCache() *CacheProvider {
	return &CacheProvider{
		Adids2Node:        make(map[string]AdIdentfier2stats),
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
type ConfigProvider interface {
	Collect() ([]check.Config, error)
	String() string
	IsUpToDate() (bool, error)
}
