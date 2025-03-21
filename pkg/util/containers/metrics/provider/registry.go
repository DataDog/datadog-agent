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
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	minRetryInterval = 2 * time.Second
)

// CollectorCatalog defines the enabled Collectors for a given runtime
type CollectorCatalog map[RuntimeMetadata]*Collectors

func (cc CollectorCatalog) merge(otherID string, othCatalog CollectorCatalog) {
	for runtime, othCollectors := range othCatalog {
		if othCollectors == nil {
			continue
		}

		currCollectors := cc[runtime]
		if currCollectors == nil {
			// We cannot use directly external `currCollectors` as it may be externally modified
			currCollectors = &Collectors{}
			cc[runtime] = currCollectors
		}

		currCollectors.merge(runtime, otherID, othCollectors)
	}
}

// CollectorMetadata contains the characteristics of a collector to be registered with RegisterCollector
type CollectorMetadata struct {
	ID         string
	Collectors CollectorCatalog
}

// CollectorFactory allows to register a factory to dynamically create Collector at startup
type CollectorFactory struct {
	ID          string
	Constructor func(*Cache, optional.Option[workloadmeta.Component]) (CollectorMetadata, error)
}

// GenericProvider offers an interface to retrieve a metrics collector
type collectorRegistry struct {
	discoveryOnce sync.Once

	catalogUpdatedCallback func(CollectorCatalog)

	registeredCollectors     map[string]CollectorFactory // key is catalogEntry.id
	registeredCollectorsLock sync.Mutex

	effectiveCollectors CollectorCatalog
}

func newCollectorRegistry() *collectorRegistry {
	registry := &collectorRegistry{
		registeredCollectors: make(map[string]CollectorFactory),
		effectiveCollectors:  make(CollectorCatalog),
	}

	return registry
}

// catalogUpdatedCallback : blocking call in the retryCollectors() function (background goroutine)
func (cr *collectorRegistry) run(c context.Context, cache *Cache, wmeta optional.Option[workloadmeta.Component], catalogUpdatedCallback func(CollectorCatalog)) {
	cr.discoveryOnce.Do(func() {
		cr.catalogUpdatedCallback = catalogUpdatedCallback

		// Always run discovery at least once synchronously
		cr.retryCollectors(cache, wmeta)

		// Now, we can run the discovery in background
		go cr.collectorDiscovery(c, cache, wmeta)
	})
}

func (cr *collectorRegistry) collectorDiscovery(c context.Context, cache *Cache, wmeta optional.Option[workloadmeta.Component]) {
	ticker := time.NewTicker(minRetryInterval)
	for {
		select {
		case <-c.Done():
			return

		case <-ticker.C:
			if remainingCollectors := cr.retryCollectors(cache, wmeta); remainingCollectors == 0 {
				log.Info("Container metrics provider discovery process finished")
				return
			}
		}
	}
}

// RegisterCollector registers a collector
func (cr *collectorRegistry) registerCollector(collectorFactory CollectorFactory) {
	cr.registeredCollectorsLock.Lock()
	defer cr.registeredCollectorsLock.Unlock()

	cr.registeredCollectors[collectorFactory.ID] = collectorFactory
}

// retryCollectors is not thread safe on purpose. It's only called by a single goroutine from `cr.run`
func (cr *collectorRegistry) retryCollectors(cache *Cache, wmeta optional.Option[workloadmeta.Component]) int {
	cr.registeredCollectorsLock.Lock()
	defer cr.registeredCollectorsLock.Unlock()

	collectorsUpdated := false
	for _, collectorFactory := range cr.registeredCollectors {
		collectorMetadata, err := collectorFactory.Constructor(cache, wmeta)
		if err == nil {
			// No need to register a collector without actual collectors
			if collectorMetadata.Collectors == nil {
				log.Debugf("Skipped registering collector %s as no collectors", collectorFactory.ID)
				continue
			}

			cr.effectiveCollectors.merge(collectorMetadata.ID, collectorMetadata.Collectors)
			collectorsUpdated = true
			delete(cr.registeredCollectors, collectorFactory.ID)
		} else {
			if errors.Is(err, ErrPermaFail) {
				delete(cr.registeredCollectors, collectorFactory.ID)
				log.Debugf("Metrics collector: %s went into PermaFail, removed from candidates", collectorFactory.ID)
			}
		}
	}

	if collectorsUpdated && cr.catalogUpdatedCallback != nil {
		cr.catalogUpdatedCallback(cr.effectiveCollectors)
	}
	return len(cr.registeredCollectors)
}

// Global registry
var registry *collectorRegistry

func init() {
	registry = newCollectorRegistry()
}

// RegisterCollector registers a collector
func RegisterCollector(collectorFactory CollectorFactory) {
	registry.registerCollector(collectorFactory)
}
