// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

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
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
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

// serviceInfo holds process data that should be cached between calls to the
// endpoint.
type serviceInfo struct {
	generatedName              string
	generatedNameSource        string
	additionalGeneratedNames   []string
	containerServiceName       string
	containerServiceNameSource string
	ddServiceName              string
	ddServiceInjected          bool
	tracerMetadata             []tracermetadata.TracerMetadata
	ports                      []uint16
	checkedContainerData       bool
	language                   string
	apmInstrumentation         string
	cmdLine                    []string
	startTimeMilli             uint64
	rss                        uint64
	cpuTime                    uint64
	cpuUsage                   float64
	containerID                string
	lastHeartbeat              int64
	addedToMap                 bool
	rxBytes                    uint64
	txBytes                    uint64
	rxBps                      float64
	txBps                      float64
}

// toModelService fills the model.Service struct pointed to by out, using the
// service info to do it.
func (i *serviceInfo) toModelService(pid int32, out *model.Service) *model.Service {
	if i == nil {
		log.Warn("toModelService called with nil pointer")
		return nil
	}

	out.PID = int(pid)
	out.GeneratedName = i.generatedName
	out.GeneratedNameSource = i.generatedNameSource
	out.AdditionalGeneratedNames = i.additionalGeneratedNames
	out.ContainerServiceName = i.containerServiceName
	out.ContainerServiceNameSource = i.containerServiceNameSource
	out.DDService = i.ddServiceName
	out.DDServiceInjected = i.ddServiceInjected
	out.TracerMetadata = i.tracerMetadata
	out.Ports = i.ports
	out.APMInstrumentation = i.apmInstrumentation
	out.Language = i.language
	out.Type = string(servicetype.Detect(i.ports))
	out.RSS = i.rss
	out.CommandLine = i.cmdLine
	out.StartTimeMilli = i.startTimeMilli
	out.CPUCores = i.cpuUsage
	out.ContainerID = i.containerID
	out.LastHeartbeat = i.lastHeartbeat
	out.RxBytes = i.rxBytes
	out.TxBytes = i.txBytes
	out.RxBps = i.rxBps
	out.TxBps = i.txBps

	return out
}

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_mock_linux.go

type timeProvider interface {
	Now() time.Time
}

type realTime struct{}

func (realTime) Now() time.Time { return time.Now() }

type pidSet map[int32]struct{}

func (s pidSet) has(pid int32) bool {
	_, present := s[pid]
	return present
}

func (s pidSet) add(pid int32) {
	s[pid] = struct{}{}
}

func (s pidSet) remove(pid int32) {
	delete(s, pid)
}

type discoveryCore struct {
	config *discoveryConfig

	// cache maps pids to data that should be cached between calls to the endpoint.
	cache map[int32]*serviceInfo

	// potentialServices stores processes that we have seen once in the previous
	// iteration, but not yet confirmed to be a running service.
	potentialServices pidSet

	// runningServices stores services that we have previously confirmed as
	// running.
	runningServices pidSet

	// ignorePids stores processes to be excluded from discovery
	ignorePids pidSet

	// lastGlobalCPUTime stores the total cpu time of the system from the last time
	// the endpoint was called.
	lastGlobalCPUTime uint64

	// lastCPUTimeUpdate is the last time lastGlobalCPUTime was updated.
	lastCPUTimeUpdate time.Time

	lastNetworkStatsUpdate time.Time

	wmeta  workloadmeta.Component
	tagger tagger.Component

	timeProvider timeProvider
	network      networkCollector

	networkErrorLimit *log.Limit
}

// cleanCache deletes dead PIDs from the cache. Note that this does not actually
// shrink the map but should free memory for the service name strings referenced
// from it. This function is not thread-safe and it is up to the caller to ensure
// proper locking.
func (c *discoveryCore) cleanCache(alivePids pidSet) {
	for pid, info := range c.cache {
		if alivePids.has(pid) {
			continue
		}

		if info.addedToMap {
			err := c.network.removePid(uint32(pid))
			if err != nil {
				log.Warn("unable to remove pid from network collector", pid, err)
			}
		}

		delete(c.cache, pid)
	}
}

// cleanPidSets deletes dead PIDs from the provided pidSets. This function is not
// thread-safe and it is up to the caller to ensure proper locking.
func (c *discoveryCore) cleanPidSets(alivePids pidSet, sets ...pidSet) {
	for _, set := range sets {
		for pid := range set {
			if alivePids.has(pid) {
				continue
			}

			delete(set, pid)
		}
	}
}

// updateNetworkStats updates the network statistics for all services in the cache.
func (c *discoveryCore) updateNetworkStats(deltaSeconds float64, response *model.ServicesResponse) {
	for pid, info := range c.cache {
		if !info.addedToMap {
			err := c.network.addPid(uint32(pid))
			if err == nil {
				info.addedToMap = true
			} else if c.networkErrorLimit.ShouldLog() {
				// This error can occur if the eBPF map used by the network
				// collector is full.
				log.Warnf("unable to add to network collector %v: %v", pid, err)
			}
			continue
		}

		stats, err := c.network.getStats(uint32(pid))
		if err != nil {
			log.Warnf("unable to get network stats %v: %v", pid, err)
			continue
		}

		deltaRx := stats.Rx - info.rxBytes
		deltaTx := stats.Tx - info.txBytes

		info.rxBps = float64(deltaRx) / deltaSeconds
		info.txBps = float64(deltaTx) / deltaSeconds

		info.rxBytes = stats.Rx
		info.txBytes = stats.Tx
	}

	updateResponseNetworkStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := c.cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.RxBps = info.rxBps
			service.TxBps = info.txBps
			service.RxBytes = info.rxBytes
			service.TxBytes = info.txBytes
		}
	}

	updateResponseNetworkStats(response.StartedServices)
	updateResponseNetworkStats(response.HeartbeatServices)
}

// maybeUpdateNetworkStats updates network statistics if enough time has passed since the last update.
func (c *discoveryCore) maybeUpdateNetworkStats(response *model.ServicesResponse) {
	if c.network == nil {
		return
	}

	now := c.timeProvider.Now()
	delta := now.Sub(c.lastNetworkStatsUpdate)
	if delta < c.config.networkStatsPeriod {
		return
	}

	deltaSeconds := delta.Seconds()

	c.updateNetworkStats(deltaSeconds, response)

	c.lastNetworkStatsUpdate = now
}

// updateServicesCPUStats updates the CPU stats of cached services, as well as the
// global CPU time cache for future updates. This function is not thread-safe and
// it is up to the caller to ensure proper locking.
func (c *discoveryCore) updateServicesCPUStats(response *model.ServicesResponse) error {
	if time.Since(c.lastCPUTimeUpdate) < c.config.cpuUsageUpdateDelay {
		return nil
	}

	globalCPUTime, err := getGlobalCPUTime()
	if err != nil {
		return fmt.Errorf("could not get global CPU time: %w", err)
	}

	for pid, info := range c.cache {
		_ = updateCPUCoresStats(int(pid), info, c.lastGlobalCPUTime, globalCPUTime)
	}

	updateResponseCPUStats := func(services []model.Service) {
		for i := range services {
			service := &services[i]
			info, ok := c.cache[int32(service.PID)]
			if !ok {
				continue
			}

			service.CPUCores = info.cpuUsage
		}
	}

	updateResponseCPUStats(response.StartedServices)
	updateResponseCPUStats(response.HeartbeatServices)

	c.lastGlobalCPUTime = globalCPUTime
	c.lastCPUTimeUpdate = time.Now()

	return nil
}

// handleStoppedServices verifies services previously seen and registered as
// running are still alive. If not, it will use the latest cached information
// about them to generate a stop event for the service. This function is not
// thread-safe and it is up to the caller to ensure proper locking.
func (c *discoveryCore) handleStoppedServices(response *model.ServicesResponse, alivePids pidSet) {
	for pid := range c.runningServices {
		if alivePids.has(pid) {
			continue
		}

		c.runningServices.remove(pid)
		info, ok := c.cache[pid]
		if !ok {
			log.Warnf("could not get service from the cache to generate a stopped service event for PID %v", pid)
			continue
		}

		// Build service struct in place in the slice
		response.StoppedServices = append(response.StoppedServices, model.Service{})
		info.toModelService(pid, &response.StoppedServices[len(response.StoppedServices)-1])
	}
}

// updateCacheInfo updates the cache with the latest heartbeat information.
func (c *discoveryCore) updateCacheInfo(response *model.ServicesResponse, now time.Time) {
	updateCachedHeartbeat := func(service *model.Service) {
		info, ok := c.cache[int32(service.PID)]
		if !ok {
			log.Warnf("could not access service info from the cache when update last heartbeat for PID %v start event", service.PID)
			return
		}

		info.lastHeartbeat = now.Unix()
		info.ports = service.Ports
		info.rss = service.RSS
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

func getServiceNameFromContainerTags(tags []string) (string, string) {
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

func (c *discoveryCore) getContainersMap() map[int]*workloadmeta.Container {
	containers := c.wmeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	containersMap := make(map[int]*workloadmeta.Container, len(containers))

	metricsProvider := metrics.GetProvider(option.New(c.wmeta))

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

func (c *discoveryCore) getProcessContainerInfo(pid int, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) (string, []string, bool) {
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
	entityTags, err := c.tagger.Tag(entityID, types.HighCardinality)
	if err != nil {
		log.Tracef("Could not get tags for container %s: %v", containerID, err)
		return containerID, collectorTags, false
	}
	tags = append(collectorTags, entityTags...)
	containerTagsCache[containerID] = tags

	return containerID, tags, true
}

func (c *discoveryCore) enrichContainerData(service *model.Service, containers map[int]*workloadmeta.Container, containerTagsCache map[string][]string) {
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

	tagName, serviceName := getServiceNameFromContainerTags(containerTags)
	service.ContainerServiceName = serviceName
	service.ContainerServiceNameSource = tagName
	service.CheckedContainerData = true

	info, ok := c.cache[int32(service.PID)]
	if ok {
		info.containerServiceName = serviceName
		info.containerServiceNameSource = tagName
		info.checkedContainerData = true
		info.containerID = containerID
	}
}

func (c *discoveryCore) close() {
	c.cleanCache(pidSet{})
	if c.network != nil {
		c.network.close()
		c.network = nil
	}
	clear(c.cache)
	clear(c.ignorePids)
	clear(c.potentialServices)
	clear(c.runningServices)
}

func (c *discoveryCore) updateRSS(response *model.ServicesResponse) {
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

func (c *discoveryCore) getServices(params params, pids []int32, context any, getService func(context any, pid int32) *model.Service) (*model.ServicesResponse, error) {
	response := &model.ServicesResponse{
		StartedServices:   make([]model.Service, 0, len(c.potentialServices)),
		StoppedServices:   make([]model.Service, 0),
		HeartbeatServices: make([]model.Service, 0),
	}

	alivePids := make(pidSet, len(pids))
	containers := c.getContainersMap()
	containerTagsCache := make(map[string][]string)

	now := c.timeProvider.Now()

	for _, pid := range pids {
		alivePids.add(pid)

		_, knownService := c.runningServices[pid]
		if knownService {
			info, ok := c.cache[pid]
			if !ok {
				// Should never happen
				continue
			}

			serviceHeartbeatTime := time.Unix(info.lastHeartbeat, 0)
			if now.Sub(serviceHeartbeatTime).Truncate(time.Minute) < params.heartbeatTime {
				// We only need to refresh the service info (ports, etc.) for
				// this service if it's time to send a heartbeat.
				continue
			}
		}

		service := getService(context, pid)
		if service == nil {
			continue
		}
		c.enrichContainerData(service, containers, containerTagsCache)

		if knownService {
			service.LastHeartbeat = now.Unix()
			response.HeartbeatServices = append(response.HeartbeatServices, *service)
			continue
		}

		if _, ok := c.potentialServices[pid]; ok {
			// We have seen it first in the previous call of getServices, so it
			// is confirmed to be running.
			c.runningServices.add(pid)
			delete(c.potentialServices, pid)
			service.LastHeartbeat = now.Unix()
			response.StartedServices = append(response.StartedServices, *service)
			continue
		}

		// This is a new potential service
		c.potentialServices.add(pid)
	}

	c.updateRSS(response)
	c.updateCacheInfo(response, now)
	c.handleStoppedServices(response, alivePids)

	c.cleanCache(alivePids)
	c.cleanPidSets(alivePids, c.ignorePids, c.potentialServices)

	if err := c.updateServicesCPUStats(response); err != nil {
		log.Warnf("updating services CPU stats: %s", err)
	}

	c.maybeUpdateNetworkStats(response)

	response.RunningServicesCount = len(c.runningServices)

	return response, nil
}
