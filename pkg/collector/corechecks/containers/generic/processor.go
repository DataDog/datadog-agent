// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	// NetworkExtensionID uniquely identifies network extensions
	NetworkExtensionID = "network"
)

// Processor contains the core logic of the generic check, allowing reusability
type Processor struct {
	metricsProvider metrics.Provider
	ctrLister       ContainerAccessor
	metricsAdapter  MetricsAdapter
	ctrFilter       ContainerFilter
	extensions      map[string]ProcessorExtension
}

// NewProcessor creates a new processor
func NewProcessor(provider metrics.Provider, lister ContainerAccessor, adapter MetricsAdapter, filter ContainerFilter) Processor {
	return Processor{
		metricsProvider: provider,
		ctrLister:       lister,
		metricsAdapter:  adapter,
		ctrFilter:       filter,
		extensions: map[string]ProcessorExtension{
			NetworkExtensionID: NewProcessorNetwork(),
		},
	}
}

// RegisterExtension allows to register (or override) an extension
func (p *Processor) RegisterExtension(id string, extension ProcessorExtension) {
	p.extensions[id] = extension
}

// Run executes the check
func (p *Processor) Run(sender aggregator.Sender, cacheValidity time.Duration) error {
	allContainers := p.ctrLister.ListRunning()

	if len(allContainers) == 0 {
		return nil
	}

	collectorsCache := make(map[workloadmeta.ContainerRuntime]metrics.Collector)
	getCollector := func(runtime workloadmeta.ContainerRuntime) metrics.Collector {
		if collector, found := collectorsCache[runtime]; found {
			return collector
		}

		collector := p.metricsProvider.GetCollector(string(runtime))
		if collector != nil {
			collectorsCache[runtime] = collector
		}
		return collector
	}

	// Extensions: PreProcess hook
	for _, extension := range p.extensions {
		extension.PreProcess(p.sendMetric, sender)
	}

	for _, container := range allContainers {
		if p.ctrFilter != nil && p.ctrFilter.IsExcluded(container) {
			log.Tracef("Container excluded due to filter, name: %s - image: %s - namespace: %s", container.Name, container.Image.Name, container.Labels[kubernetes.CriContainerNamespaceLabel])
			continue
		}

		entityID := containers.BuildTaggerEntityName(container.ID)
		tags, err := tagger.Tag(entityID, collectors.HighCardinality)
		if err != nil {
			log.Errorf("Could not collect tags for container %q, err: %v", container.ID[:12], err)
			continue
		}
		tags = p.metricsAdapter.AdaptTags(tags, container)

		collector := getCollector(container.Runtime)
		if collector == nil {
			log.Warnf("Collector not found for container: %v, metrics will ne missing", container)
			continue
		}

		containerStats, err := collector.GetContainerStats(container.Namespace, container.ID, cacheValidity)
		if err != nil {
			log.Debugf("Container stats for: %v not available through collector %q, err: %v", container, collector.ID(), err)
			continue
		}

		if err := p.processContainer(sender, tags, container, containerStats); err != nil {
			log.Debugf("Generating metrics for container: %v failed, metrics may be missing, err: %v", container, err)
			continue
		}

		openFiles, err := collector.GetContainerOpenFilesCount(container.Namespace, container.ID, cacheValidity)
		if err == nil {
			p.sendMetric(sender.Gauge, "container.pid.open_files", pointer.UIntPtrToFloatPtr(openFiles), tags)
		} else {
			log.Debugf("OpenFiles count for: %v not available through collector %q, err: %v", container, collector.ID(), err)
		}

		// TODO: Implement container stats. We currently don't have enough information from Metadata service to do it.

		// Extensions: Process hook
		for _, extension := range p.extensions {
			extension.Process(tags, container, collector, cacheValidity)
		}
	}

	// Extensions: PostProcess hook
	for _, extension := range p.extensions {
		extension.PostProcess()
	}

	sender.Commit()
	return nil
}

func (p *Processor) processContainer(sender aggregator.Sender, tags []string, container *workloadmeta.Container, containerStats *metrics.ContainerStats) error {
	if uptime := time.Since(container.State.StartedAt); uptime >= 0 {
		p.sendMetric(sender.Gauge, "container.uptime", pointer.Ptr(uptime.Seconds()), tags)
	}

	if containerStats == nil {
		log.Debugf("Metrics provider returned nil stats for container: %v", container)
		return nil
	}

	if containerStats.CPU != nil {
		p.sendMetric(sender.Rate, "container.cpu.usage", containerStats.CPU.Total, tags)
		p.sendMetric(sender.Rate, "container.cpu.user", containerStats.CPU.User, tags)
		p.sendMetric(sender.Rate, "container.cpu.system", containerStats.CPU.System, tags)
		p.sendMetric(sender.Rate, "container.cpu.throttled", containerStats.CPU.ThrottledTime, tags)
		p.sendMetric(sender.Rate, "container.cpu.throttled.periods", containerStats.CPU.ThrottledPeriods, tags)
		// Convert CPU Limit to nanoseconds to allow easy percentage computation in the App.
		if containerStats.CPU.Limit != nil {
			p.sendMetric(sender.Gauge, "container.cpu.limit", pointer.Ptr(*containerStats.CPU.Limit*float64(time.Second/100)), tags)
		}
	}

	if containerStats.Memory != nil {
		p.sendMetric(sender.Gauge, "container.memory.usage", containerStats.Memory.UsageTotal, tags)
		p.sendMetric(sender.Gauge, "container.memory.kernel", containerStats.Memory.KernelMemory, tags)
		p.sendMetric(sender.Gauge, "container.memory.limit", containerStats.Memory.Limit, tags)
		p.sendMetric(sender.Gauge, "container.memory.soft_limit", containerStats.Memory.Softlimit, tags)
		p.sendMetric(sender.Gauge, "container.memory.rss", containerStats.Memory.RSS, tags)
		p.sendMetric(sender.Gauge, "container.memory.cache", containerStats.Memory.Cache, tags)
		p.sendMetric(sender.Gauge, "container.memory.swap", containerStats.Memory.Swap, tags)
		p.sendMetric(sender.Gauge, "container.memory.oom_events", containerStats.Memory.OOMEvents, tags)
		p.sendMetric(sender.Gauge, "container.memory.working_set", containerStats.Memory.PrivateWorkingSet, tags)
		p.sendMetric(sender.Gauge, "container.memory.commit", containerStats.Memory.CommitBytes, tags)
		p.sendMetric(sender.Gauge, "container.memory.commit.peak", containerStats.Memory.CommitPeakBytes, tags)
	}

	if containerStats.IO != nil {
		for deviceName, deviceStats := range containerStats.IO.Devices {
			deviceTags := taggerUtils.ConcatenateStringTags(tags, "device:"+deviceName, "device_name:"+deviceName)
			p.sendMetric(sender.Rate, "container.io.read", deviceStats.ReadBytes, deviceTags)
			p.sendMetric(sender.Rate, "container.io.read.operations", deviceStats.ReadOperations, deviceTags)
			p.sendMetric(sender.Rate, "container.io.write", deviceStats.WriteBytes, deviceTags)
			p.sendMetric(sender.Rate, "container.io.write.operations", deviceStats.WriteOperations, deviceTags)
		}

		if len(containerStats.IO.Devices) == 0 {
			p.sendMetric(sender.Rate, "container.io.read", containerStats.IO.ReadBytes, tags)
			p.sendMetric(sender.Rate, "container.io.read.operations", containerStats.IO.ReadOperations, tags)
			p.sendMetric(sender.Rate, "container.io.write", containerStats.IO.WriteBytes, tags)
			p.sendMetric(sender.Rate, "container.io.write.operations", containerStats.IO.WriteOperations, tags)
		}
	}

	if containerStats.PID != nil {
		p.sendMetric(sender.Gauge, "container.pid.thread_count", containerStats.PID.ThreadCount, tags)
		p.sendMetric(sender.Gauge, "container.pid.thread_limit", containerStats.PID.ThreadLimit, tags)
	}

	return nil
}

func (p *Processor) sendMetric(senderFunc func(string, float64, string, []string), metricName string, value *float64, tags []string) {
	if value == nil {
		return
	}

	metricName, val := p.metricsAdapter.AdaptMetrics(metricName, *value)
	if metricName != "" {
		senderFunc(metricName, val, "", tags)
	}
}
