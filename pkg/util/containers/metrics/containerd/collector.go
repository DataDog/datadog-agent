// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && (linux || windows)

package containerd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl/v2"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	//nolint:revive // TODO(CINT) Fix revive linter
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "containerd"
	collectorPriority = 1

	pidCacheGCInterval     = 60 * time.Second
	pidCacheFullRefreshKey = "refreshTime"
)

func init() {
	provider.RegisterCollector(provider.CollectorFactory{
		ID: collectorID,
		Constructor: func(cache *provider.Cache, _ optional.Option[workloadmeta.Component]) (provider.CollectorMetadata, error) {
			return newContainerdCollector(cache)
		},
	})
}

type containerdCollector struct {
	client   cutil.ContainerdItf
	pidCache *provider.Cache
}

func newContainerdCollector(cache *provider.Cache) (provider.CollectorMetadata, error) {
	var collectorMetadata provider.CollectorMetadata

	if !config.IsFeaturePresent(config.Containerd) {
		return collectorMetadata, provider.ErrPermaFail
	}

	client, err := cutil.NewContainerdUtil()
	if err != nil {
		return collectorMetadata, provider.ConvertRetrierErr(err)
	}

	collector := &containerdCollector{
		client:   client,
		pidCache: provider.NewCache(pidCacheGCInterval),
	}

	collectors := &provider.Collectors{
		Stats:             provider.MakeRef[provider.ContainerStatsGetter](collector, collectorPriority),
		Network:           provider.MakeRef[provider.ContainerNetworkStatsGetter](collector, collectorPriority),
		PIDs:              provider.MakeRef[provider.ContainerPIDsGetter](collector, collectorPriority),
		ContainerIDForPID: provider.MakeRef[provider.ContainerIDForPIDRetriever](collector, collectorPriority),
	}

	kataCollectors := &provider.Collectors{
		Stats: provider.MakeRef[provider.ContainerStatsGetter](collector, collectorPriority),
	}

	return provider.CollectorMetadata{
		ID: collectorID,
		Collectors: provider.CollectorCatalog{
			provider.NewRuntimeMetadata(string(provider.RuntimeNameContainerd), ""):                                 provider.MakeCached(collectorID, cache, collectors),
			provider.NewRuntimeMetadata(string(provider.RuntimeNameContainerd), string(provider.RuntimeFlavorKata)): provider.MakeCached(collectorID, cache, kataCollectors),
		},
	}, nil
}

// GetContainerStats returns stats by container ID.
func (c *containerdCollector) GetContainerStats(containerNS, containerID string, _ time.Duration) (*provider.ContainerStats, error) {
	metrics, err := c.getContainerdMetrics(containerNS, containerID)
	if err != nil {
		return nil, err
	}

	containerStats, err := processContainerStats(containerID, metrics)
	if err != nil {
		return nil, err
	}

	// We got the main stats, best effort to fill remaining fields
	container, err := c.client.Container(containerNS, containerID)
	if err != nil {
		log.Debugf("Could not fetch container with ID %s: %v", containerID, err)
		return containerStats, nil
	}

	// Filling information from Spec
	var OCISpec *oci.Spec
	info, err := c.client.Info(containerNS, container)
	if err == nil {
		OCISpec, err = c.client.Spec(containerNS, info, containerd.DefaultAllowedSpecMaxSize)
	}

	if err == nil {
		fillStatsFromSpec(containerStats, OCISpec)
	} else {
		log.Debugf("could not retrieve OCI Spec from container with ID %s: %v", containerID, err)
	}

	return containerStats, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (c *containerdCollector) GetContainerNetworkStats(containerNS, containerID string, _ time.Duration) (*provider.ContainerNetworkStats, error) {
	metrics, err := c.getContainerdMetrics(containerNS, containerID)
	if err != nil {
		return nil, err
	}

	return processContainerNetworkStats(containerID, metrics)
}

// GetPIDs returns the list of PIDs by container ID.
func (c *containerdCollector) GetPIDs(containerNS, containerID string, _ time.Duration) ([]int, error) {
	container, err := c.client.Container(containerNS, containerID)
	if err != nil {
		log.Debugf("Could not fetch container with ID %s: %v", containerID, err)
		return nil, err
	}

	processes, err := c.client.TaskPids(containerNS, container)
	if err != nil || len(processes) == 0 {
		log.Debugf("Unable to get TaskPids for containerwith ID %s: %v", containerID, err)
		return nil, err
	}

	pids := make([]int, len(processes))
	for _, process := range processes {
		pids = append(pids, int(process.Pid))
	}
	return pids, nil
}

// GetContainerIDForPID returns the container ID for given PID
func (c *containerdCollector) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	currentTime := time.Now()
	strPid := strconv.Itoa(pid)

	cID, found, _ := c.pidCache.Get(currentTime, strPid, cacheValidity)
	if found {
		return cID.(string), nil
	}

	if err := c.refreshPIDCache(currentTime, cacheValidity); err != nil {
		return "", err
	}

	// Use harcoded cacheValidity as input one could be 0
	cID, found, _ = c.pidCache.Get(currentTime, strPid, time.Second)
	if found {
		return cID.(string), nil
	}

	return "", nil
}

// This method returns interface{} because the metrics could be an instance of
// v1.Metrics (for Linux) or stats.Statistics (Windows) and they don't share a
// common interface.
func (c *containerdCollector) getContainerdMetrics(containerNS string, containerID string) (interface{}, error) {
	container, err := c.client.Container(containerNS, containerID)
	if err != nil {
		return nil, fmt.Errorf("could not get container with ID %s: %s", containerID, err)
	}

	metricTask, errTask := c.client.TaskMetrics(containerNS, container)
	if errTask != nil {
		return nil, fmt.Errorf("could not get metrics for container with ID %s: %s", containerID, err)
	}

	metrics, err := typeurl.UnmarshalAny(&anypb.Any{
		TypeUrl: metricTask.Data.TypeUrl,
		Value:   metricTask.Data.Value,
	})
	if err != nil {
		return nil, fmt.Errorf("could not convert the metrics data from container with ID %s: %s", containerID, err)
	}

	return metrics, nil
}

func (c *containerdCollector) refreshPIDCache(currentTime time.Time, cacheValidity time.Duration) error {
	// If we've done a full refresh within cacheValidity, we do not trigger another full refresh
	// We're using the cache itself with a dedicated key pidCacheFullRefreshKey to know if
	// we need to perform a full refresh or not to seamlessly handle cacheValidity and cache GC.
	_, found, err := c.pidCache.Get(currentTime, pidCacheFullRefreshKey, cacheValidity)
	if found {
		return err
	}

	// Full refresh
	namespaces, err := cutil.NamespacesToWatch(context.TODO(), c.client)
	if err != nil {
		c.pidCache.Store(currentTime, pidCacheFullRefreshKey, struct{}{}, err)
		return err
	}

	for _, namespace := range namespaces {
		containers, err := c.client.Containers(namespace)
		if err != nil {
			c.pidCache.Store(currentTime, pidCacheFullRefreshKey, struct{}{}, err)
			return err
		}

		for _, container := range containers {
			processes, err := c.client.TaskPids(namespace, container)
			if err != nil {
				log.Debugf("Could not retrieve the processes of the container with ID %s: %s", container.ID(), err)
			}

			for _, process := range processes {
				c.pidCache.Store(currentTime, strconv.FormatUint(uint64(process.Pid), 10), container.ID(), nil)
			}
		}
	}

	c.pidCache.Store(currentTime, pidCacheFullRefreshKey, struct{}{}, nil)
	return nil
}
