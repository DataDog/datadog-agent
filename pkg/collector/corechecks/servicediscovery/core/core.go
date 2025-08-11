// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/servicetype"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// Use a low cache validity to ensure that we refresh information every time
	// the check is run if needed. This is the same as cacheValidityNoRT in
	// pkg/process/checks/container.go.
	containerCacheValidity = 2 * time.Second
)

// ServiceInfo holds process data that should be cached between calls to the
// endpoint.
type ServiceInfo struct {
	model.Service
	CheckedContainerData bool
	CPUTime              uint64
}

// ToModelService fills the model.Service struct pointed to by out, using the
// service info to do it.
func (i *ServiceInfo) ToModelService(pid int32, out *model.Service) *model.Service {
	if i == nil {
		log.Warn("ToModelService called with nil pointer")
		return nil
	}

	*out = i.Service
	out.PID = int(pid)
	out.Type = string(servicetype.Detect(i.Ports))

	return out
}

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_mock_linux.go

// TimeProvider defines an interface for getting the current time.
type TimeProvider interface {
	Now() time.Time
}

// RealTime provides the real system time.
type RealTime struct{}

// Now returns the current system time.
func (RealTime) Now() time.Time { return time.Now() }

// PidSet represents a set of process IDs.
type PidSet map[int32]struct{}

// Has returns true if the set contains the given pid.
func (s PidSet) Has(pid int32) bool {
	_, present := s[pid]
	return present
}

// Add adds the given pid to the set.
func (s PidSet) Add(pid int32) {
	s[pid] = struct{}{}
}

// Remove removes the given pid from the set.
func (s PidSet) Remove(pid int32) {
	delete(s, pid)
}

// Discovery represents the core service discovery functionality.
type Discovery struct {
	Config *DiscoveryConfig

	// Cache maps pids to data that should be cached between calls to the endpoint.
	Cache map[int32]*ServiceInfo

	// PotentialServices stores processes that we have seen once in the previous
	// iteration, but not yet confirmed to be a running service.
	PotentialServices PidSet

	// RunningServices stores services that we have previously confirmed as
	// running.
	RunningServices PidSet

	// IgnorePids stores processes to be excluded from discovery
	IgnorePids PidSet

	// LastGlobalCPUTime stores the total cpu time of the system from the last time
	// the endpoint was called.
	LastGlobalCPUTime uint64

	// LastCPUTimeUpdate is the last time lastGlobalCPUTime was updated.
	LastCPUTimeUpdate time.Time

	LastNetworkStatsUpdate time.Time

	WMeta  workloadmeta.Component
	Tagger tagger.Component

	TimeProvider TimeProvider
	Network      NetworkCollector

	NetworkErrorLimit *log.Limit
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it. This function is not thread-safe and it is up to the caller to ensure
// proper locking.
func (c *Discovery) cleanCache(alivePids PidSet) {
	for pid := range c.Cache {
		if alivePids.Has(pid) {
			continue
		}

		delete(c.Cache, pid)
	}
}

// cleanPidSets deletes dead PIDs from the provided pidSets. This function is not
// thread-safe and it is up to the caller to ensure proper locking.
func (c *Discovery) cleanPidSets(alivePids PidSet, sets ...PidSet) {
	for _, set := range sets {
		for pid := range set {
			if alivePids.Has(pid) {
				continue
			}

			delete(set, pid)
		}
	}
}

// updateNetworkStats updates the network statistics for all services in the cache.
func (c *Discovery) updateNetworkStats(deltaSeconds float64, response *model.ServicesResponse) {
	pids := make(PidSet, len(c.Cache))
	for pid := range c.Cache {
		pids.Add(pid)
	}

	allStats, err := c.Network.GetStats(pids)
	if err != nil {
		log.Warnf("unable to get network stats: %v", err)
		return
	}

	for pid, stats := range allStats {
		info, ok := c.Cache[int32(pid)]
		if !ok {
			continue
		}

		deltaRx := stats.Rx - info.RxBytes
		deltaTx := stats.Tx - info.TxBytes

		info.RxBps = float64(deltaRx) / deltaSeconds
		info.TxBps = float64(deltaTx) / deltaSeconds

		info.RxBytes = stats.Rx
		info.TxBytes = stats.Tx
	}

	updateResponseNetworkStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := c.Cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.RxBps = info.RxBps
			service.TxBps = info.TxBps
			service.RxBytes = info.RxBytes
			service.TxBytes = info.TxBytes
		}
	}

	updateResponseNetworkStats(response.StartedServices)
	updateResponseNetworkStats(response.HeartbeatServices)
}

// maybeUpdateNetworkStats updates network statistics if enough time has passed since the last update.
func (c *Discovery) maybeUpdateNetworkStats(response *model.ServicesResponse) {
	if c.Network == nil {
		return
	}

	now := c.TimeProvider.Now()
	delta := now.Sub(c.LastNetworkStatsUpdate)
	if delta < c.Config.NetworkStatsPeriod {
		return
	}

	deltaSeconds := delta.Seconds()

	c.updateNetworkStats(deltaSeconds, response)

	c.LastNetworkStatsUpdate = now
}

// updateServicesCPUStats updates the CPU stats of cached services, as well as the
// global CPU time cache for future updates. This function is not thread-safe and
// it is up to the caller to ensure proper locking.
func (c *Discovery) updateServicesCPUStats(response *model.ServicesResponse) error {
	if time.Since(c.LastCPUTimeUpdate) < c.Config.CPUUsageUpdateDelay {
		return nil
	}

	globalCPUTime, err := getGlobalCPUTime()
	if err != nil {
		return fmt.Errorf("could not get global CPU time: %w", err)
	}

	for pid, info := range c.Cache {
		_ = updateCPUCoresStats(int(pid), info, c.LastGlobalCPUTime, globalCPUTime)
	}

	updateResponseCPUStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := c.Cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.CPUCores = info.CPUCores
		}
	}

	updateResponseCPUStats(response.StartedServices)
	updateResponseCPUStats(response.HeartbeatServices)

	c.LastGlobalCPUTime = globalCPUTime
	c.LastCPUTimeUpdate = time.Now()

	return nil
}

// handleStoppedServices verifies services previously seen and registered as
// running are still alive. If not, it will use the latest cached information
// about them to generate a stop event for the service. This function is not
// thread-safe and it is up to the caller to ensure proper locking.
func (c *Discovery) handleStoppedServices(response *model.ServicesResponse, alivePids PidSet) {
	for pid := range c.RunningServices {
		if alivePids.Has(pid) {
			continue
		}

		c.RunningServices.Remove(pid)
		info, ok := c.Cache[pid]
		if !ok {
			log.Warnf("could not get service from the cache to generate a stopped service event for PID %v", pid)
			continue
		}

		// Build service struct in place in the slice
		response.StoppedServices = append(response.StoppedServices, model.Service{})
		info.ToModelService(pid, &response.StoppedServices[len(response.StoppedServices)-1])
	}
}

// updateCacheInfo updates the cache with the latest heartbeat information.
func (c *Discovery) updateCacheInfo(response *model.ServicesResponse, now time.Time) {
	updateCachedHeartbeat := func(service *model.Service) {
		info, ok := c.Cache[int32(service.PID)]
		if !ok {
			log.Warnf("could not access service info from the cache when update last heartbeat for PID %v start event", service.PID)
			return
		}

		info.LastHeartbeat = now.Unix()
		info.Ports = service.Ports
		info.RSS = service.RSS
	}

	for i := range response.StartedServices {
		service := &response.StartedServices[i]
		updateCachedHeartbeat(service)
	}

	for i := range response.HeartbeatServices {
		service := &response.HeartbeatServices[i]
		updateCachedHeartbeat(service)
	}
}

// GetServiceNameFromContainerTags extracts service name information from container tags.
func GetServiceNameFromContainerTags(tags []string) (string, string) {
	// The tags we look for service name generation, in their priority order.
	// The map entries will be filled as we go through the containers tags.
	tagsPriority := []struct {
		tagName  string
		tagValue *string
	}{
		{"service", nil},
		{"app", nil},
		{"short_image", nil},
		{"kube_container_name", nil},
		{"kube_deployment", nil},
		{"kube_service", nil},
	}

	// Sort the tags to make the function deterministic
	slices.Sort(tags)

	for _, tag := range tags {
		// Get index of separator between name and value
		sepIndex := strings.IndexRune(tag, ':')
		if sepIndex < 0 || sepIndex >= len(tag)-1 {
			// Malformed tag; we skip it
			continue
		}

		for i := range tagsPriority {
			if tagsPriority[i].tagValue != nil {
				// We have seen this tag before, we don't need another value.
				continue
			}

			if tag[:sepIndex] != tagsPriority[i].tagName {
				// Not a tag we care about; we skip it
				continue
			}

			value := tag[sepIndex+1:]
			tagsPriority[i].tagValue = &value
			break
		}
	}

	for _, tag := range tagsPriority {
		if tag.tagValue == nil {
			continue
		}

		log.Debugf("Using %v:%v tag for service name", tag.tagName, *tag.tagValue)
		return tag.tagName, *tag.tagValue
	}

	return "", ""
}

// GetContainersMap returns a map of container information.
func (c *Discovery) GetContainersMap() map[int]*workloadmeta.Container {
	containers := c.WMeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	containersMap := make(map[int]*workloadmeta.Container, len(containers))

	metricsProvider := metrics.GetProvider(option.New(c.WMeta))

	for _, container := range containers {
		collector := metricsProvider.GetCollector(provider.NewRuntimeMetadata(
			string(container.Runtime),
			string(container.RuntimeFlavor),
		))
		if collector == nil {
			containersMap[int(container.PID)] = container
			continue
		}

		pids, err := collector.GetPIDs(container.Namespace, container.ID, containerCacheValidity)
		if err != nil || len(pids) == 0 {
			containersMap[int(container.PID)] = container
			continue
		}

		for _, pid := range pids {
			containersMap[int(pid)] = container
		}
	}
	return containersMap
}

func (c *Discovery) getProcessContainerInfo(pid int, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) (string, []string, bool) {
	container, ok := containers[pid]
	if !ok {
		return "", nil, false
	}

	tags, ok := containerTagsCache[container.EntityID.ID]
	if ok {
		return container.EntityID.ID, tags, true
	}

	containerID := container.EntityID.ID
	collectorTags := container.CollectorTags

	// Getting the tags from the tagger. This logic is borrowed from
	// the GetContainers helper in pkg/process/util/containers.
	entityID := types.NewEntityID(types.ContainerID, containerID)
	entityTags, err := c.Tagger.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Tracef("Could not get tags for container %s: %v", containerID, err)
		return containerID, collectorTags, false
	}
	tags = append(collectorTags, entityTags...)
	containerTagsCache[containerID] = tags

	return containerID, tags, true
}

// EnrichContainerData adds container information to a service.
func (c *Discovery) EnrichContainerData(service *model.Service, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) {
	containerID, containerTags, ok := c.getProcessContainerInfo(service.PID, containers, containerTagsCache)
	if !ok {
		return
	}

	service.ContainerID = containerID
	service.ContainerTags = containerTags

	// We checked the container tags before, no need to do it again.
	if service.CheckedContainerData {
		return
	}

	tagName, serviceName := GetServiceNameFromContainerTags(containerTags)
	service.ContainerServiceName = serviceName
	service.ContainerServiceNameSource = tagName
	service.CheckedContainerData = true

	info, ok := c.Cache[int32(service.PID)]
	if ok {
		info.ContainerServiceName = serviceName
		info.ContainerServiceNameSource = tagName
		info.CheckedContainerData = true
		info.ContainerID = containerID
	}
}

// Close cleans up resources used by the Discovery instance.
func (c *Discovery) Close() {
	c.cleanCache(PidSet{})
	if c.Network != nil {
		c.Network.Close()
		c.Network = nil
	}
	clear(c.Cache)
	clear(c.IgnorePids)
	clear(c.PotentialServices)
	clear(c.RunningServices)
}

func (c *Discovery) updateRSS(response *model.ServicesResponse) {
	updateResponseRSS := func(services []model.Service) {
		for i := range services {
			service := &services[i]

			rss, err := getRSS(int32(service.PID))
			if err != nil {
				continue
			}

			service.RSS = rss
		}
	}

	updateResponseRSS(response.StartedServices)
	updateResponseRSS(response.HeartbeatServices)
}

// GetServices retrieves service information based on the provided parameters.
func (c *Discovery) GetServices(params Params, pids []int32, context any, getService func(context any, pid int32) *model.Service) (*model.ServicesResponse, error) {
	response := &model.ServicesResponse{
		StartedServices:   make([]model.Service, 0, len(c.PotentialServices)),
		StoppedServices:   make([]model.Service, 0),
		HeartbeatServices: make([]model.Service, 0),
	}

	alivePids := make(PidSet, len(pids))
	containers := c.GetContainersMap()
	containerTagsCache := make(map[string][]string)

	now := c.TimeProvider.Now()

	for _, pid := range pids {
		alivePids.Add(pid)

		_, knownService := c.RunningServices[pid]
		if knownService {
			info, ok := c.Cache[pid]
			if !ok {
				// Should never happen
				continue
			}

			serviceHeartbeatTime := time.Unix(info.LastHeartbeat, 0)
			if now.Sub(serviceHeartbeatTime).Truncate(time.Minute) < params.HeartbeatTime {
				// We only need to refresh the service info (ports, etc.) for
				// this service if it's time to send a heartbeat.
				continue
			}
		}

		service := getService(context, pid)
		if service == nil {
			continue
		}
		c.EnrichContainerData(service, containers, containerTagsCache)

		if knownService {
			service.LastHeartbeat = now.Unix()
			response.HeartbeatServices = append(response.HeartbeatServices, *service)
			continue
		}

		if _, ok := c.PotentialServices[pid]; ok {
			// We have seen it first in the previous call of getServices, so it
			// is confirmed to be running.
			c.RunningServices.Add(pid)
			delete(c.PotentialServices, pid)
			service.LastHeartbeat = now.Unix()
			response.StartedServices = append(response.StartedServices, *service)
			continue
		}

		// This is a new potential service
		c.PotentialServices.Add(pid)
	}

	c.updateRSS(response)
	c.updateCacheInfo(response, now)
	c.handleStoppedServices(response, alivePids)

	c.cleanCache(alivePids)
	c.cleanPidSets(alivePids, c.IgnorePids, c.PotentialServices)

	if err := c.updateServicesCPUStats(response); err != nil {
		log.Warnf("updating services CPU stats: %s", err)
	}

	c.maybeUpdateNetworkStats(response)

	response.RunningServicesCount = len(c.RunningServices)

	return response, nil
}
