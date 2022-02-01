// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri
// +build cri

package cri

import (
	"time"

	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	criCollectorID    = "cri"
	criCacheKeyPrefix = "cri-stats-"
	criCacheTTL       = 10 * time.Second
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       criCollectorID,
		Priority: 1, // Less than the "system" collector, so we can rely on cgroups directly if possible
		Runtimes: []string{provider.RuntimeNameCRIO},
		Factory: func() (provider.Collector, error) {
			return newCRICollector()
		},
	})
}

type criCollector struct {
	client         cri.CRIClient
	lastScrapeTime time.Time
}

func newCRICollector() (*criCollector, error) {
	if !config.IsFeaturePresent(config.Cri) {
		return nil, provider.ErrPermaFail
	}

	client, err := cri.GetUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &criCollector{client: client}, nil
}

// ID returns the collector ID.
func (collector *criCollector) ID() string {
	return criCollectorID
}

// GetContainerStats returns stats by container ID.
func (collector *criCollector) GetContainerStats(containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := collector.getCriContainerStats(containerID, cacheValidity)
	if err != nil {
		return nil, err
	}

	return &provider.ContainerStats{
		Timestamp: time.Now(),
		CPU: &provider.ContainerCPUStats{
			Total: util.UIntToFloatPtr(stats.GetCpu().GetUsageCoreNanoSeconds().GetValue()),
		},
		Memory: &provider.ContainerMemStats{
			RSS: util.UIntToFloatPtr(stats.GetMemory().GetWorkingSetBytes().GetValue()),
		},
	}, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (collector *criCollector) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	// Not available
	return nil, nil
}

func (collector *criCollector) getCriContainerStats(containerID string, cacheValidity time.Duration) (*v1alpha2.ContainerStats, error) {
	refreshRequired := collector.lastScrapeTime.Add(cacheValidity).Before(time.Now())
	cacheKey := criCacheKeyPrefix + containerID
	if cachedMetrics, found := cache.Cache.Get(cacheKey); found && !refreshRequired {
		log.Debugf("Got CRI stats from cache for container %s", containerID)
		return cachedMetrics.(*v1alpha2.ContainerStats), nil
	}

	stats, err := collector.client.GetContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	collector.lastScrapeTime = time.Now()
	cache.Cache.Set(cacheKey, stats, criCacheTTL)

	return stats, nil
}
