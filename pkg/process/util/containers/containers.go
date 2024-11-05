// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package containers

import (
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	floatNanoseconds float64 = float64(time.Second)
)

// ContainerRateMetrics holds previous values for a container,
// in order to compute rates
type ContainerRateMetrics struct {
	ContainerStatsTimestamp time.Time
	NetworkStatsTimestamp   time.Time
	UserCPU                 float64
	SystemCPU               float64
	TotalCPU                float64
	IOReadBytes             float64
	IOWriteBytes            float64
	NetworkRcvdBytes        float64
	NetworkSentBytes        float64
	NetworkRcvdPackets      float64
	NetworkSentPackets      float64
}

// NullContainerRates can be safely used for containers that have no
// previous rate values stored (new containers)
var NullContainerRates = ContainerRateMetrics{
	UserCPU:   -1,
	SystemCPU: -1,
	TotalCPU:  -1,
}

var (
	initContainerProvider   sync.Once
	sharedContainerProvider ContainerProvider
)

// ContainerProvider defines the interface for a container metrics provider
type ContainerProvider interface {
	GetContainers(cacheValidity time.Duration, previousContainers map[string]*ContainerRateMetrics) ([]*model.Container, map[string]*ContainerRateMetrics, map[int]string, error)
	GetPidToCid(cacheValidity time.Duration) map[int]string
}

// InitSharedContainerProvider init shared ContainerProvider
func InitSharedContainerProvider(wmeta workloadmeta.Component, tagger tagger.Component) ContainerProvider {
	initContainerProvider.Do(func() {
		sharedContainerProvider = NewDefaultContainerProvider(wmeta, tagger)
	})
	return sharedContainerProvider
}

// GetSharedContainerProvider returns a shared ContainerProvider
func GetSharedContainerProvider() (ContainerProvider, error) {
	if sharedContainerProvider == nil {
		return nil, log.Errorf("Shared container provider not initialized")
	}
	return sharedContainerProvider, nil
}

// containerProvider provides data about containers usable by process-agent
type containerProvider struct {
	metricsProvider metrics.Provider
	metadataStore   workloadmeta.Component
	filter          *containers.Filter
	tagger          tagger.Component
}

// NewContainerProvider returns a ContainerProvider instance
func NewContainerProvider(provider metrics.Provider, metadataStore workloadmeta.Component, filter *containers.Filter, tagger tagger.Component) ContainerProvider {
	return &containerProvider{
		metricsProvider: provider,
		metadataStore:   metadataStore,
		filter:          filter,
		tagger:          tagger,
	}
}

// NewDefaultContainerProvider returns a ContainerProvider built with default metrics provider and metadata provider
func NewDefaultContainerProvider(wmeta workloadmeta.Component, tagger tagger.Component) ContainerProvider {
	containerFilter, err := containers.GetSharedMetricFilter()
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
	}

	// TODO(components): stop relying on globals and use injected components instead whenever possible.
	return NewContainerProvider(metrics.GetProvider(optional.NewOption(wmeta)), wmeta, containerFilter, tagger)
}

// GetContainers returns containers found on the machine
func (p *containerProvider) GetContainers(cacheValidity time.Duration, previousContainers map[string]*ContainerRateMetrics) ([]*model.Container, map[string]*ContainerRateMetrics, map[int]string, error) {
	containersMetadata := p.metadataStore.ListContainersWithFilter(workloadmeta.GetRunningContainers)

	processContainers := make([]*model.Container, 0)
	rateStats := make(map[string]*ContainerRateMetrics)
	pidToCid := make(map[int]string)
	for _, container := range containersMetadata {
		var annotations map[string]string
		if pod, err := p.metadataStore.GetKubernetesPodForContainer(container.ID); err == nil {
			annotations = pod.Annotations
		}

		if p.filter != nil && p.filter.IsExcluded(annotations, container.Name, container.Image.Name, container.Labels[kubernetes.CriContainerNamespaceLabel]) {
			continue
		}

		if container.Runtime == workloadmeta.ContainerRuntimeGarden && len(container.CollectorTags) == 0 {
			log.Debugf("No tags found for garden container: %s, skipping", container.ID)
			continue
		}

		entityID := types.NewEntityID(types.ContainerID, container.ID)
		tags, err := p.tagger.Tag(entityID, types.HighCardinality)
		if err != nil {
			log.Debugf("Could not collect tags for container %q, err: %v", container.ID[:12], err)
		}
		tags = append(tags, container.CollectorTags...)

		outPreviousStats := NullContainerRates
		// Name and Image fields exist but are never filled
		processContainer := &model.Container{
			Type:       convertContainerRuntime(container.Runtime),
			Id:         container.ID,
			Started:    container.State.StartedAt.Unix(),
			Created:    container.State.CreatedAt.Unix(),
			Tags:       tags,
			State:      convertContainerStatus(container.State.Status),
			Health:     convertHealthStatus(container.State.Health),
			Addresses:  computeContainerAddrs(container),
			RepoDigest: container.Image.RepoDigest,
		}

		// Always adding container if we have metadata as we do want to report containers without stats
		processContainers = append(processContainers, processContainer)

		// Gathering container & network statistics
		previousContainerRates := previousContainers[container.ID]
		if previousContainerRates == nil {
			previousContainerRates = &NullContainerRates
		}

		collector := p.metricsProvider.GetCollector(provider.NewRuntimeMetadata(
			string(container.Runtime),
			string(container.RuntimeFlavor),
		))
		if collector == nil {
			log.Infof("No metrics collector available for runtime: %s, skipping container: %s", container.Runtime, container.ID)
			continue
		}

		containerStats, err := collector.GetContainerStats(container.Namespace, container.ID, cacheValidity)
		if err != nil || containerStats == nil {
			log.Debugf("Container stats for: %+v not available, err: %v", container, err)
			// If main container stats are missing, we skip the container
			continue
		}
		computeContainerStats(container, containerStats, previousContainerRates, &outPreviousStats, processContainer)

		// Building PID to CID mapping for NPM
		pids, err := collector.GetPIDs(container.Namespace, container.ID, cacheValidity)
		if err == nil && pids != nil {
			for _, pid := range pids {
				pidToCid[pid] = container.ID
			}
		} else {
			log.Debugf("PIDs for: %+v not available, err: %v", container, err)
		}

		containerNetworkStats, err := collector.GetContainerNetworkStats(container.Namespace, container.ID, cacheValidity)
		if err != nil {
			log.Debugf("Container network stats for: %+v not available, err: %v", container, err)
		}
		computeContainerNetworkStats(containerNetworkStats, previousContainerRates, &outPreviousStats, processContainer)

		// Storing previous stats
		rateStats[processContainer.Id] = &outPreviousStats
	}

	return processContainers, rateStats, pidToCid, nil
}

// GetPidToCid returns containers found on the machine
func (p *containerProvider) GetPidToCid(cacheValidity time.Duration) map[int]string {
	containersMetadata := p.metadataStore.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	pidToCid := make(map[int]string)
	for _, container := range containersMetadata {
		var annotations map[string]string
		if pod, err := p.metadataStore.GetKubernetesPodForContainer(container.ID); err == nil {
			annotations = pod.Annotations
		}

		if p.filter != nil && p.filter.IsExcluded(annotations, container.Name, container.Image.Name, container.Labels[kubernetes.CriContainerNamespaceLabel]) {
			continue
		}

		collector := p.metricsProvider.GetCollector(provider.NewRuntimeMetadata(
			string(container.Runtime),
			string(container.RuntimeFlavor),
		))
		if collector == nil {
			log.Infof("No metrics collector available for runtime: %s, skipping container: %s", container.Runtime, container.ID)
			continue
		}

		// Building PID to CID mapping for NPM and Language Detection
		pids, err := collector.GetPIDs(container.Namespace, container.ID, cacheValidity)
		if err == nil && pids != nil {
			for _, pid := range pids {
				pidToCid[pid] = container.ID
			}
		} else {
			log.Debugf("PIDs for: %+v not available, err: %v", container, err)
		}
	}

	return pidToCid
}

func computeContainerStats(container *workloadmeta.Container, inStats *metrics.ContainerStats, previousStats, outPreviousStats *ContainerRateMetrics, outStats *model.Container) {
	if inStats == nil {
		return
	}

	// All collectors should provide timestamped data, logging to trace these issues
	if inStats.Timestamp.IsZero() {
		log.Debug("Missing timestamp in container stats - use current timestamp")
		inStats.Timestamp = time.Now()
	}
	outPreviousStats.ContainerStatsTimestamp = inStats.Timestamp

	if inStats.CPU != nil {
		outPreviousStats.TotalCPU = statValue(inStats.CPU.Total, -1)
		outPreviousStats.UserCPU = statValue(inStats.CPU.User, -1)
		outPreviousStats.SystemCPU = statValue(inStats.CPU.System, -1)

		outStats.TotalPct = float32(cpuRatePctValue(outPreviousStats.TotalCPU, previousStats.TotalCPU, inStats.Timestamp, previousStats.ContainerStatsTimestamp))
		outStats.UserPct = float32(cpuRatePctValue(outPreviousStats.UserCPU, previousStats.UserCPU, inStats.Timestamp, previousStats.ContainerStatsTimestamp))
		outStats.SystemPct = float32(cpuRatePctValue(outPreviousStats.SystemCPU, previousStats.SystemCPU, inStats.Timestamp, previousStats.ContainerStatsTimestamp))
		outStats.CpuUsageNs = float32(cpuRateValue(outPreviousStats.TotalCPU, previousStats.TotalCPU, inStats.Timestamp, previousStats.ContainerStatsTimestamp))

		// We only emit limit if it was not defaulted
		if !inStats.CPU.DefaultedLimit {
			outStats.CpuLimit = float32(statValue(inStats.CPU.Limit, 0))
		}

		// Values taken from container manifest
		outStats.CpuRequest = float32(statValue(container.Resources.CPURequest, 0))
	}

	if inStats.Memory != nil {
		outStats.MemoryLimit = uint64(statValue(inStats.Memory.Limit, 0))
		outStats.MemCache = uint64(statValue(inStats.Memory.Cache, 0))
		outStats.MemRss = uint64(statValue(inStats.Memory.RSS, 0))
		outStats.MemUsage = uint64(statValue(inStats.Memory.UsageTotal, 0))

		// On Linux OOM Killer (memory limit) uses ~WorkingSet, on Windows it's CommitBytes
		if inStats.Memory.WorkingSet != nil {
			outStats.MemAccounted = uint64(*inStats.Memory.WorkingSet)
		} else if inStats.Memory.CommitBytes != nil {
			outStats.MemAccounted = uint64(*inStats.Memory.CommitBytes)
		}

		// Values taken from container manifest
		if container.Resources.MemoryRequest != nil {
			outStats.MemoryRequest = *container.Resources.MemoryRequest
		}
	}

	if inStats.PID != nil {
		outStats.ThreadCount = uint64(statValue(inStats.PID.ThreadCount, 0))
		outStats.ThreadLimit = uint64(statValue(inStats.PID.ThreadLimit, 0))
	}

	if inStats.IO != nil {
		outPreviousStats.IOReadBytes = statValue(inStats.IO.ReadBytes, 0)
		outPreviousStats.IOWriteBytes = statValue(inStats.IO.WriteBytes, 0)

		outStats.Rbps = float32(rateValue(outPreviousStats.IOReadBytes, previousStats.IOReadBytes, inStats.Timestamp, previousStats.ContainerStatsTimestamp))
		outStats.Wbps = float32(rateValue(outPreviousStats.IOWriteBytes, previousStats.IOWriteBytes, inStats.Timestamp, previousStats.ContainerStatsTimestamp))
	}
}

func computeContainerNetworkStats(inStats *metrics.ContainerNetworkStats, previousStats, outPreviousStats *ContainerRateMetrics, outStats *model.Container) {
	if inStats == nil {
		return
	}

	// All collectors should provide timestamped data, logging to trace these issues
	if inStats.Timestamp.IsZero() {
		log.Debug("Missing timestamp in container stats - use current timestamp")
		inStats.Timestamp = time.Now()
	}
	outPreviousStats.NetworkStatsTimestamp = inStats.Timestamp

	outPreviousStats.NetworkRcvdBytes = statValue(inStats.BytesRcvd, 0)
	outPreviousStats.NetworkSentBytes = statValue(inStats.BytesSent, 0)
	outPreviousStats.NetworkRcvdPackets = statValue(inStats.PacketsRcvd, 0)
	outPreviousStats.NetworkSentPackets = statValue(inStats.PacketsSent, 0)

	outStats.NetRcvdBps = float32(rateValue(outPreviousStats.NetworkRcvdBytes, previousStats.NetworkRcvdBytes, inStats.Timestamp, previousStats.NetworkStatsTimestamp))
	outStats.NetSentBps = float32(rateValue(outPreviousStats.NetworkSentBytes, previousStats.NetworkSentBytes, inStats.Timestamp, previousStats.NetworkStatsTimestamp))
	outStats.NetRcvdPs = float32(rateValue(outPreviousStats.NetworkRcvdPackets, previousStats.NetworkRcvdPackets, inStats.Timestamp, previousStats.NetworkStatsTimestamp))
	outStats.NetSentPs = float32(rateValue(outPreviousStats.NetworkSentPackets, previousStats.NetworkSentPackets, inStats.Timestamp, previousStats.NetworkStatsTimestamp))
}

func computeContainerAddrs(container *workloadmeta.Container) []*model.ContainerAddr {
	if len(container.NetworkIPs) == 0 || len(container.Ports) == 0 {
		return nil
	}

	addrs := make([]*model.ContainerAddr, 0, len(container.NetworkIPs)*len(container.Ports))
	for _, containerIP := range container.NetworkIPs {
		for _, port := range container.Ports {
			addrs = append(addrs, &model.ContainerAddr{
				Ip:       containerIP,
				Port:     int32(port.Port),
				Protocol: model.ConnectionType(model.ConnectionType_value[port.Protocol]),
			})
		}
	}
	return addrs
}

func convertContainerRuntime(runtime workloadmeta.ContainerRuntime) string {
	// ECSFargate is special and used to be mapped to "ECS"
	if runtime == workloadmeta.ContainerRuntimeECSFargate {
		return "ECS"
	}

	return string(runtime)
}

func convertHealthStatus(health workloadmeta.ContainerHealth) model.ContainerHealth {
	// This works because unknown keys will return 0 (which is unknown)
	return model.ContainerHealth(model.ContainerHealth_value[string(health)])
}

func convertContainerStatus(status workloadmeta.ContainerStatus) model.ContainerState {
	if status == workloadmeta.ContainerStatusStopped {
		return model.ContainerState_exited
	}

	return model.ContainerState(model.ContainerState_value[string(status)])
}

func statValue(val *float64, def float64) float64 {
	if val != nil {
		return *val
	}

	return def
}

func cpuRateValue(current, previous float64, currentTs, previousTs time.Time) float64 {
	if current == -1 || previous == -1 {
		return -1
	}

	return rateValue(current, previous, currentTs, previousTs)
}

func cpuRatePctValue(current, previous float64, currentTs, previousTs time.Time) float64 {
	if current == -1 || previous == -1 {
		return -1
	}

	return 100 * rateValue(current, previous, currentTs, previousTs) / floatNanoseconds
}

func rateValue(current, previous float64, currentTs, previousTs time.Time) float64 {
	if previousTs.IsZero() {
		return 0
	}

	timeDiff := currentTs.Sub(previousTs).Seconds()
	if timeDiff <= 0 {
		return 0
	}

	valueDiff := current - previous
	if valueDiff <= 0 {
		return 0
	}

	return valueDiff / timeDiff
}
