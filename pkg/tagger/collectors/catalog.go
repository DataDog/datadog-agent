// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

// CollectorFactory is functions that return a Collector
type CollectorFactory func() Collector

// Catalog holds available collectors for detection and usage
type Catalog map[string]CollectorFactory

// DefaultCatalog holds every compiled-in collector
var DefaultCatalog = make(Catalog)

// registerCollector is to be called by collectors to be added to the default catalog
func registerCollector(name string, c CollectorFactory) {
	DefaultCatalog[name] = c
}
