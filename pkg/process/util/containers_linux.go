// +build linux

package util

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	containerCacheDuration = 10 * time.Second
	detector               *collectors.Detector
)

// SetContainerSource allows config to force a single container source
func SetContainerSource(name string) {
	detector = collectors.NewDetector(name)
}

// GetContainers returns containers found on the machine, autodetecting
// the best backend from available sources
func GetContainers() ([]*containers.Container, error) {
	// Detect source
	if detector == nil {
		detector = collectors.NewDetector("")
	}
	l, name, err := detector.GetPreferred()
	if err != nil {
		return nil, err
	}

	// Get containers from cache and update metrics
	cacheKey := cache.BuildAgentKey("containers", name)
	cached, hit := cache.Cache.Get(cacheKey)
	if hit {
		containers, ok := cached.([]*containers.Container)
		if ok {
			err := l.UpdateMetrics(containers)
			log.Tracef("Got %d containers from cache", len(containers))
			return containers, err
		}
		log.Errorf("Invalid container list cache format, forcing a cache miss")
	}

	// If cache empty/expired, get a new container list
	containers, err := l.List()
	if err != nil {
		return nil, err
	}
	cache.Cache.Set(cacheKey, containers, containerCacheDuration)
	log.Tracef("Got %d containers from source %s", len(containers), name)
	return containers, nil
}

// ExtractContainerRateMetric extracts relevant rate values from a container list
// for later reuse, while reducing memory usage to only the needed fields
func ExtractContainerRateMetric(containers []*containers.Container) map[string]ContainerRateMetrics {
	out := make(map[string]ContainerRateMetrics)
	for _, c := range containers {
		m := ContainerRateMetrics{
			CPU:        c.CPU,
			IO:         c.IO,
			NetworkSum: c.Network.SumInterfaces(),
		}
		out[c.ID] = m
	}
	return out
}
