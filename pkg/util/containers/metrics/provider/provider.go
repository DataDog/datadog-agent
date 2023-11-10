// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package provider defines the Provider interface which allows to get metrics
// collectors for the different container runtimes supported (Docker,
// containerd, etc.).
package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Runtime is a typed string for supported container runtimes
type Runtime string

// Known container runtimes
const (
	RuntimeNameDocker     Runtime = "docker"
	RuntimeNameContainerd Runtime = "containerd"
	RuntimeNameCRIO       Runtime = "cri-o"
	RuntimeNameGarden     Runtime = "garden"
	RuntimeNamePodman     Runtime = "podman"
	RuntimeNameECSFargate Runtime = "ecsfargate"
)

var (
	// ErrNothingYet is returned when no collector is currently detected.
	// This might change in the future if new collectors are valid.
	ErrNothingYet = &retry.Error{
		LogicError:    errors.New("no collector detected for runtime"),
		RessourceName: "catalog",
		RetryStatus:   retry.FailWillRetry,
	}

	// ErrPermaFail is returned when a collector will never be available
	ErrPermaFail = &retry.Error{
		LogicError:    errors.New("no collector available for runtime"),
		RessourceName: "catalog",
		RetryStatus:   retry.PermaFail,
	}

	// AllLinuxRuntimes lists all runtimes available on Linux
	// nolint: deadcode, unused
	AllLinuxRuntimes = []Runtime{
		RuntimeNameDocker,
		RuntimeNameContainerd,
		RuntimeNameCRIO,
		RuntimeNameGarden,
		RuntimeNamePodman,
		RuntimeNameECSFargate,
	}

	// AllWindowsRuntimes lists all runtimes available on Windows
	// nolint: deadcode, unused
	AllWindowsRuntimes = []Runtime{
		RuntimeNameDocker,
		RuntimeNameContainerd,
		RuntimeNameECSFargate,
	}
)

// Provider interface allows to mock the metrics provider
type Provider interface {
	GetCollector(runtime string) Collector
	GetMetaCollector() MetaCollector
}

var (
	metricsProvider     *GenericProvider
	initMetricsProvider sync.Once
)

// GenericProvider offers an interface to retrieve a metrics collector
type GenericProvider struct {
	collectors    map[Runtime]*collectorImpl
	cache         *Cache
	metaCollector *metaCollector
}

// GetProvider returns the metrics provider singleton
func GetProvider() Provider {
	initMetricsProvider.Do(func() {
		metricsProvider = newProvider()
	})

	return metricsProvider
}

func newProvider() *GenericProvider {
	provider := &GenericProvider{
		cache:         NewCache(cacheGCInterval),
		metaCollector: newMetaCollector(),
	}
	registry.run(context.TODO(), provider.cache, provider.collectorsUpdatedCallback)

	return provider
}

// GetCollector returns the best collector for given runtime.
// The best collector may change depending on other collectors availability.
// You should not cache the result from this function.
func (mp *GenericProvider) GetCollector(runtime string) Collector {
	return mp.collectors[Runtime(runtime)]
}

// GetMetaCollector returns the meta collector.
func (mp *GenericProvider) GetMetaCollector() MetaCollector {
	return mp.metaCollector
}

func (mp *GenericProvider) collectorsUpdatedCallback(collectorsCatalog CollectorCatalog) {
	// Update local collectors
	newCollectors := make(map[Runtime]*collectorImpl, len(collectorsCatalog))
	for runtime, collectors := range collectorsCatalog {
		newCollectors[runtime] = fromCollectors(collectors)
	}

	mp.collectors = newCollectors

	// Update metacollectors
	mp.metaCollector.collectorsUpdatedCallback(collectorsCatalog)
}
