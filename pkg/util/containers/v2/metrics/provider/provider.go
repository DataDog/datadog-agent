// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Known container runtimes
const (
	RuntimeNameDocker     string = "docker"
	RuntimeNameContainerd string = "containerd"
	RuntimeNameCRIO       string = "cri-o"
	RuntimeNameGarden     string = "garden"
)

const (
	minRetryInterval = 2 * time.Second
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
	AllLinuxRuntimes = []string{
		RuntimeNameDocker,
		RuntimeNameContainerd,
		RuntimeNameCRIO,
	}
	// AllWindowsRuntimes lists all runtimes available on Windows
	// nolint: deadcode, unused
	AllWindowsRuntimes = []string{
		RuntimeNameDocker,
		RuntimeNameContainerd,
	}
)

// Provider interface allows to mock the metrics provider
type Provider interface {
	GetCollector(runtime string) Collector
	RegisterCollector(collectorMeta CollectorMetadata)
}

type collectorFactory func() (Collector, error)

// CollectorMetadata contains the characteristics of a collector to be registered with RegisterCollector
type CollectorMetadata struct {
	ID            string
	Priority      int // lowest gets higher priority (0 more prioritary than 1)
	Runtimes      []string
	Factory       collectorFactory
	DelegateCache bool
}

type collectorReference struct {
	id        string
	priority  int
	collector Collector
}

// GenericProvider offers an interface to retrieve a metrics collector
type GenericProvider struct {
	collectors          map[string]CollectorMetadata // key is catalogEntry.id
	collectorsLock      sync.Mutex
	effectiveCollectors map[string]*collectorReference // key is runtime
	effectiveLock       sync.RWMutex
	lastRetryTimestamp  time.Time
	remainingCandidates uint32
}

var metricsProvider = newProvider()

// GetProvider returns the metrics provider singleton
func GetProvider() Provider {
	return metricsProvider
}

func newProvider() *GenericProvider {
	return &GenericProvider{
		collectors:          make(map[string]CollectorMetadata),
		effectiveCollectors: make(map[string]*collectorReference),
	}
}

// GetCollector returns the best collector for given runtime.
// The best collector may change depending on other collectors availability.
// You should not cache the result from this function.
func (mp *GenericProvider) GetCollector(runtime string) Collector {
	mp.retryCollectors(minRetryInterval)
	return mp.getCollector(runtime)
}

// RegisterCollector registers a collector
func (mp *GenericProvider) RegisterCollector(collectorMeta CollectorMetadata) {
	mp.collectorsLock.Lock()
	defer mp.collectorsLock.Unlock()

	mp.collectors[collectorMeta.ID] = collectorMeta
	atomic.StoreUint32(&mp.remainingCandidates, uint32(len(mp.collectors)))
}

func (mp *GenericProvider) getCollector(runtime string) Collector {
	mp.effectiveLock.RLock()
	defer mp.effectiveLock.RUnlock()

	if entry, found := mp.effectiveCollectors[runtime]; found {
		return entry.collector
	}

	return nil
}

func (mp *GenericProvider) retryCollectors(cacheValidity time.Duration) {
	if atomic.LoadUint32(&mp.remainingCandidates) == 0 {
		return
	}

	mp.collectorsLock.Lock()
	defer mp.collectorsLock.Unlock()

	// Only refresh if last attempt is too old (incl. processing time)
	if time.Now().Before(mp.lastRetryTimestamp.Add(cacheValidity)) {
		return
	}

	mp.lastRetryTimestamp = time.Now()

	for _, collectorEntry := range mp.collectors {
		collector, err := collectorEntry.Factory()
		if err == nil {
			if collectorEntry.DelegateCache {
				collector = NewCollectorCache(collector)
			}

			mp.updateEffectiveCollectors(collector, collectorEntry)
			delete(mp.collectors, collectorEntry.ID)
		} else {
			if errors.Is(err, ErrPermaFail) {
				delete(mp.collectors, collectorEntry.ID)
				log.Debugf("Metrics collector: %s went into PermaFail, removed from candidates", collectorEntry.ID)
			}
		}
	}

	atomic.StoreUint32(&mp.remainingCandidates, uint32(len(mp.collectors)))
}

func (mp *GenericProvider) updateEffectiveCollectors(newCollector Collector, newCollectorDesc CollectorMetadata) {
	mp.effectiveLock.Lock()
	defer mp.effectiveLock.Unlock()

	newRef := collectorReference{
		id:        newCollectorDesc.ID,
		priority:  newCollectorDesc.Priority,
		collector: newCollector,
	}

	for _, runtime := range newCollectorDesc.Runtimes {
		currentCollector := mp.effectiveCollectors[runtime]
		if currentCollector == nil {
			log.Infof("Using metrics collector: %s for runtime: %s", newRef.id, runtime)
			mp.effectiveCollectors[runtime] = &newRef
		} else if currentCollector.priority > newCollectorDesc.Priority { // do not replace on same priority to favor consistency
			log.Infof("Replaced old collector: %s by new collector: %s for runtime: %s", currentCollector.id, newRef.id, runtime)
			mp.effectiveCollectors[runtime] = &newRef
		}
	}
}
