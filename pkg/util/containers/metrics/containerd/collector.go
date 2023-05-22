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
	"github.com/containerd/typeurl"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containerd"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	containerdCollectorID = "containerd"

	pidCacheGCInterval     = 60 * time.Second
	pidCacheFullRefreshKey = "refreshTime"
)

func init() {
	provider.GetProvider().RegisterCollector(provider.CollectorMetadata{
		ID:       containerdCollectorID,
		Priority: 1, // Less than the "system" collector, so we can rely on cgroups directly if possible
		Runtimes: []string{provider.RuntimeNameContainerd},
		Factory: func() (provider.Collector, error) {
			return newContainerdCollector()
		},
		DelegateCache: true,
	})
}

type containerdCollector struct {
	client            cutil.ContainerdItf
	workloadmetaStore workloadmeta.Store
	pidCache          *provider.Cache
}

func newContainerdCollector() (*containerdCollector, error) {
	if !config.IsFeaturePresent(config.Containerd) {
		return nil, provider.ErrPermaFail
	}

	client, err := cutil.NewContainerdUtil()
	if err != nil {
		return nil, provider.ConvertRetrierErr(err)
	}

	return &containerdCollector{
		client:            client,
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		pidCache:          provider.NewCache(pidCacheGCInterval),
	}, nil
}

// ID returns the collector ID.
func (c *containerdCollector) ID() string {
	return containerdCollectorID
}

// GetContainerStats returns stats by container ID.
func (c *containerdCollector) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerStats, error) {
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

	// Filling the PIDs if returned
	processes, err := c.client.TaskPids(containerNS, container)
	if err == nil {
		if len(processes) > 0 {
			if containerStats.PID == nil {
				containerStats.PID = &provider.ContainerPIDStats{
					PIDs: make([]int, len(processes)),
				}
			}

			for _, process := range processes {
				containerStats.PID.PIDs = append(containerStats.PID.PIDs, int(process.Pid))
			}
		}
	} else {
		log.Debugf("Unable to get TaskPids for containerwith ID %s: %v", containerID, err)
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

// GetContainerOpenFilesCount returns open files count by container ID.
func (c *containerdCollector) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	// Not available
	return nil, nil
}

// GetContainerNetworkStats returns network stats by container ID.
func (c *containerdCollector) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*provider.ContainerNetworkStats, error) {
	metrics, err := c.getContainerdMetrics(containerNS, containerID)
	if err != nil {
		return nil, err
	}

	return processContainerNetworkStats(containerID, metrics)
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

// GetSelfContainerID returns current process container ID
func (c *containerdCollector) GetSelfContainerID() (string, error) {
	// Not available
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

	metrics, err := typeurl.UnmarshalAny(metricTask.Data)
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
