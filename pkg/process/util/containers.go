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
	detectors              []*collectors.Detector
	dedupe                 = false
)

// SetContainerSources allows config to force one or multiple container sources
func SetContainerSources(names []string) {
	detectors = []*collectors.Detector{}
	for _, name := range names {
		detectors = append(detectors, collectors.NewDetector(name))
	}
	dedupe = len(detectors) > 1
}

// GetContainers returns containers found on the machine
// GetContainers autodetects the best backend from available sources
// if the users don't specify the preferred container sources
func GetContainers() ([]*containers.Container, error) {
	// Detect sources
	if detectors == nil {
		// Container sources aren't configured, autodetect the best available source
		detectors = []*collectors.Detector{collectors.NewDetector("")}
	}

	result := []*containers.Container{}
	for _, detector := range detectors {
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
				if err != nil {
					log.Debugf("Cannot update container metrics via %s: %s", name, err)
					continue
				}
				log.Tracef("Got %d containers from cache", len(containers))
				result = append(result, containers...)
				continue
			}
			log.Errorf("Invalid container list cache format, forcing a cache miss")
		}

		// If cache empty/expired, get a new container list
		containers, err := l.List()
		if err != nil {
			log.Errorf("Cannot list containers via %s: %s", name, err)
			continue
		}
		cache.Cache.Set(cacheKey, containers, containerCacheDuration)
		log.Tracef("Got %d containers from source %s", len(containers), name)
		result = append(result, containers...)
	}

	if dedupe {
		return dedupeContainers(result), nil
	}
	return result, nil
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

// dedupeContainers remove duplicated containers in a slice
func dedupeContainers(ctrs []*containers.Container) []*containers.Container {
	m := map[string]*containers.Container{}
	deduped := []*containers.Container{}
	for _, ctr := range ctrs {
		m[ctr.ID] = ctr
	}
	for _, ctr := range m {
		deduped = append(deduped, ctr)
	}
	return deduped
}
