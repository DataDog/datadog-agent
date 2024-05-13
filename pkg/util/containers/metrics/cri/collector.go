// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"time"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	collectorID       = "cri"
	collectorPriority = 3
)

func init() {
	provider.RegisterCollector(provider.CollectorFactory{
		ID: collectorID,
		Constructor: func(cache *provider.Cache, _ optional.Option[workloadmeta.Component]) (provider.CollectorMetadata, error) {
			return newCRICollector(cache)
		},
	})
}

type criCollector struct {
	client cri.CRIClient
}

func newCRICollector(cache *provider.Cache) (provider.CollectorMetadata, error) {
	var collectorMetadata provider.CollectorMetadata

	if !config.IsFeaturePresent(config.Cri) {
		return collectorMetadata, provider.ErrPermaFail
	}

	client, err := cri.GetUtil()
	if err != nil {
		return collectorMetadata, provider.ConvertRetrierErr(err)
	}

	collector := &criCollector{client: client}
	collectors := &provider.Collectors{
		Stats: provider.MakeRef[provider.ContainerStatsGetter](collector, collectorPriority),
	}

	return provider.CollectorMetadata{
		ID: collectorID,
		Collectors: provider.CollectorCatalog{
			provider.NewRuntimeMetadata(string(provider.RuntimeNameCRIO), ""): provider.MakeCached(collectorID, cache, collectors),
		},
	}, nil
}

// GetContainerStats returns stats by container ID.
//
//nolint:revive // TODO(CINT) Fix revive linter
func (collector *criCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
	stats, err := collector.getCriContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	containerStats := &provider.ContainerStats{}

	if stats.Cpu != nil {
		containerStats.Timestamp = time.Unix(0, stats.Cpu.Timestamp)
		containerStats.CPU = &provider.ContainerCPUStats{
			Total: convertRuntimeUInt64Value(stats.Cpu.UsageCoreNanoSeconds),
		}
	}

	if stats.Memory != nil {
		containerStats.Timestamp = time.Unix(0, stats.Memory.Timestamp)
		containerStats.Memory = &provider.ContainerMemStats{
			UsageTotal: convertRuntimeUInt64Value(stats.Memory.UsageBytes),
			WorkingSet: convertRuntimeUInt64Value(stats.Memory.WorkingSetBytes),
			RSS:        convertRuntimeUInt64Value(stats.Memory.RssBytes),
		}
	}

	return containerStats, nil
}

func (collector *criCollector) getCriContainerStats(containerID string) (*v1.ContainerStats, error) {
	stats, err := collector.client.GetContainerStats(containerID)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func convertRuntimeUInt64Value(v *v1.UInt64Value) *float64 {
	if v == nil {
		return nil
	}

	return pointer.Ptr(float64(v.GetValue()))
}
