// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package cpuoscillation implements the per-container CPU oscillation detection check.
// This check detects rapid CPU cycling patterns on a per-container basis, helping SREs
// identify specific workloads exhibiting unhealthy behavior like restart loops or thrashing.
package cpuoscillation

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "cpu_oscillation"

	// emitInterval is how often we emit metrics (every 15 seconds)
	emitInterval = 15 * time.Second
)

// ContainerDetector wraps OscillationDetector with container-specific state
// REQ-COD-002: Per-container detector with lifecycle management
// REQ-COD-004: ~500 bytes per container
type ContainerDetector struct {
	detector      *OscillationDetector
	containerID   string
	namespace     string // Container namespace (for metrics provider)
	runtime       string // Container runtime
	runtimeFlavor string // Runtime flavor

	// CPU rate calculation (same pattern as pkg/process/util/containers)
	lastCPUTotal   float64
	lastSampleTime time.Time
}

// Check implements the per-container CPU oscillation detection check
// REQ-COD-001: Detect Rapid CPU Cycling Per Container
// REQ-COD-002: Establish Container-Specific Baseline
// REQ-COD-003: Report Oscillation Characteristics with Container Tags
// REQ-COD-004: Minimal Performance Impact at Scale
// REQ-COD-005: Configurable Detection with Default Disabled
// REQ-COD-006: Metric Emission for All Tracked Containers
// REQ-COD-007: Graceful Error Handling
type Check struct {
	core.CheckBase

	// Per-container detector map
	detectors   map[string]*ContainerDetector
	detectorsMu sync.RWMutex

	// Shared configuration
	config *Config

	// Component dependencies
	wmeta           workloadmeta.Component
	tagger          tagger.Component
	metricsProvider metrics.Provider

	// Lifecycle management
	stopCh       chan struct{}
	wmetaEventCh chan workloadmeta.EventBundle
}

// Factory returns a new check factory
// REQ-COD-005: Default disabled - requires explicit opt-in via configuration
func Factory(store workloadmeta.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase:       core.NewCheckBase(CheckName),
			config:          &Config{},
			detectors:       make(map[string]*ContainerDetector),
			wmeta:           store,
			tagger:          tagger,
			metricsProvider: metrics.GetProvider(option.New(store)),
			stopCh:          make(chan struct{}),
		})
	})
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := c.config.Parse(config); err != nil {
		return err
	}

	// REQ-COD-005: Default disabled - exit early if not enabled
	if !c.config.Enabled {
		return errors.New("cpu_oscillation check is disabled by default; set enabled: true in configuration to enable")
	}

	log.Infof("[%s] Configured with enabled=%t, amplitude_multiplier=%.2f, min_amplitude=%.2f, min_direction_reversals=%d, warmup_seconds=%d",
		CheckName, c.config.Enabled, c.config.AmplitudeMultiplier, c.config.MinAmplitude, c.config.MinDirectionReversals, c.config.WarmupSeconds)

	return nil
}

// Run starts the CPU oscillation check - runs indefinitely until stopped
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	// REQ-COD-002: Subscribe to WorkloadMeta for container lifecycle events
	c.subscribeToWorkloadMeta()
	defer c.wmeta.Unsubscribe(c.wmetaEventCh)

	// Initialize detectors for existing containers
	c.initializeExistingContainers()

	// REQ-COD-004: 1Hz sampling
	sampleTicker := time.NewTicker(time.Second)
	defer sampleTicker.Stop()

	// Emit metrics every 15s
	emitTicker := time.NewTicker(emitInterval)
	defer emitTicker.Stop()

	for {
		select {
		case eventBundle := <-c.wmetaEventCh:
			// REQ-COD-002: Handle container lifecycle events
			eventBundle.Acknowledge()
			for _, event := range eventBundle.Events {
				c.handleWorkloadMetaEvent(event)
			}

		case <-sampleTicker.C:
			// REQ-COD-001: Sample CPU for all containers at 1Hz
			c.sampleAllContainers()

		case <-emitTicker.C:
			// REQ-COD-006: Emit metrics for all containers
			c.emitMetrics()

		case <-c.stopCh:
			return nil
		}
	}
}

// subscribeToWorkloadMeta sets up subscription to container lifecycle events
// REQ-COD-002: Container lifecycle management via WorkloadMeta
func (c *Check) subscribeToWorkloadMeta() {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		Build()

	c.wmetaEventCh = c.wmeta.Subscribe(
		"cpu_oscillation",
		workloadmeta.NormalPriority,
		filter,
	)
}

// handleWorkloadMetaEvent processes container creation and removal events
// REQ-COD-002: Create/delete detectors on container start/stop
func (c *Check) handleWorkloadMetaEvent(event workloadmeta.Event) {
	container, ok := event.Entity.(*workloadmeta.Container)
	if !ok {
		return
	}

	switch event.Type {
	case workloadmeta.EventTypeSet:
		// Container created or updated
		if container.State.Running {
			c.ensureDetector(container)
		} else {
			// Container stopped but not removed - clean up state
			c.removeDetector(container.ID)
		}
	case workloadmeta.EventTypeUnset:
		// REQ-COD-002: Container removed - immediate state cleanup
		c.removeDetector(container.ID)
	}
}

// initializeExistingContainers creates detectors for all running containers at startup
func (c *Check) initializeExistingContainers() {
	containers := c.wmeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
	for _, container := range containers {
		c.ensureDetector(container)
	}
	log.Debugf("[%s] Initialized detectors for %d existing containers", CheckName, len(containers))
}

// ensureDetector creates a detector for a container if one doesn't exist
// REQ-COD-002: New container = new detector with fresh warmup
func (c *Check) ensureDetector(container *workloadmeta.Container) {
	c.detectorsMu.Lock()
	defer c.detectorsMu.Unlock()

	if _, exists := c.detectors[container.ID]; exists {
		return // Already tracking
	}

	c.detectors[container.ID] = &ContainerDetector{
		detector:      NewOscillationDetector(c.config.DetectorConfig()),
		containerID:   container.ID,
		namespace:     container.Namespace,
		runtime:       string(container.Runtime),
		runtimeFlavor: string(container.RuntimeFlavor),
		lastCPUTotal:  -1, // Sentinel for "no previous sample"
	}
	log.Debugf("[%s] Created detector for container %s", CheckName, container.ID[:12])
}

// removeDetector removes a container's detector state
// REQ-COD-002: Immediate state cleanup on container removal
func (c *Check) removeDetector(containerID string) {
	c.detectorsMu.Lock()
	defer c.detectorsMu.Unlock()

	if _, exists := c.detectors[containerID]; exists {
		delete(c.detectors, containerID)
		shortID := containerID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		log.Debugf("[%s] Removed detector for container %s", CheckName, shortID)
	}
}

// sampleAllContainers samples CPU for all tracked containers
// REQ-COD-001: Sample CPU at 1Hz
// REQ-COD-007: Skip and continue on errors
func (c *Check) sampleAllContainers() {
	c.detectorsMu.RLock()
	detectorsCopy := make([]*ContainerDetector, 0, len(c.detectors))
	for _, cd := range c.detectors {
		detectorsCopy = append(detectorsCopy, cd)
	}
	c.detectorsMu.RUnlock()

	for _, cd := range detectorsCopy {
		cpuPercent, err := c.sampleContainerCPU(cd)
		if err != nil {
			// REQ-COD-007: Log at debug level (transient errors are expected)
			shortID := cd.containerID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			log.Debugf("[%s] Failed to sample CPU for container %s: %v", CheckName, shortID, err)
			continue
		}
		cd.detector.AddSample(cpuPercent)

		// REQ-COD-002: Update warmup timer
		cd.detector.DecrementWarmup()
	}
}

var errFirstSample = errors.New("first sample, need delta")

// sampleContainerCPU gets CPU usage for a container using the metrics provider
// REQ-COD-004: Use container metrics provider (cgroup-based)
func (c *Check) sampleContainerCPU(cd *ContainerDetector) (float64, error) {
	collector := c.metricsProvider.GetCollector(provider.NewRuntimeMetadata(
		cd.runtime,
		cd.runtimeFlavor,
	))
	if collector == nil {
		return 0, fmt.Errorf("no collector for runtime %s", cd.runtime)
	}

	stats, err := collector.GetContainerStats(cd.namespace, cd.containerID, 0)
	if err != nil {
		// REQ-COD-007: Return error to be handled gracefully
		return 0, fmt.Errorf("failed to get container stats: %w", err)
	}

	if stats == nil || stats.CPU == nil || stats.CPU.Total == nil {
		return 0, errors.New("no CPU stats available")
	}

	// CPU.Total is in nanoseconds (cumulative)
	currentTotal := *stats.CPU.Total
	currentTime := stats.Timestamp
	if currentTime.IsZero() {
		currentTime = time.Now()
	}

	// First sample - need delta
	if cd.lastCPUTotal < 0 || cd.lastSampleTime.IsZero() {
		cd.lastCPUTotal = currentTotal
		cd.lastSampleTime = currentTime
		return 0, errFirstSample
	}

	// Calculate CPU percentage since last sample
	timeDelta := currentTime.Sub(cd.lastSampleTime)
	if timeDelta <= 0 {
		return 0, errors.New("no time elapsed")
	}

	cpuDelta := currentTotal - cd.lastCPUTotal
	if cpuDelta < 0 {
		// Counter reset (container restarted)
		cd.lastCPUTotal = currentTotal
		cd.lastSampleTime = currentTime
		return 0, errors.New("CPU counter reset")
	}

	// Convert to percentage: (cpu_ns_used / elapsed_ns) * 100
	cpuPercent := (cpuDelta / float64(timeDelta.Nanoseconds())) * 100.0

	cd.lastCPUTotal = currentTotal
	cd.lastSampleTime = currentTime

	return cpuPercent, nil
}

// emitMetrics analyzes and emits oscillation metrics for all containers
// REQ-COD-003: Emit metrics with container tags
// REQ-COD-006: Emit for ALL containers regardless of oscillation state
func (c *Check) emitMetrics() {
	sender, err := c.GetSender()
	if err != nil {
		log.Warnf("[%s] Error getting sender: %v", CheckName, err)
		return
	}

	c.detectorsMu.RLock()
	defer c.detectorsMu.RUnlock()

	// REQ-COD-007: Handle no containers gracefully (not an error)
	if len(c.detectors) == 0 {
		log.Debugf("[%s] No containers to emit metrics for", CheckName)
		return
	}

	for containerID, cd := range c.detectors {
		// Don't emit until window is full
		if !cd.detector.IsWindowFull() {
			continue
		}

		// REQ-COD-003: Get container tags via tagger (respects DD_CHECKS_TAG_CARDINALITY)
		entityID := types.NewEntityID(types.ContainerID, containerID)
		tags, err := c.tagger.Tag(entityID, types.ChecksConfigCardinality)
		if err != nil {
			shortID := containerID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			// REQ-COD-007: Continue with empty tags on tagger failure
			log.Debugf("[%s] Failed to get tags for container %s: %v", CheckName, shortID, err)
			tags = []string{}
		}

		result := cd.detector.Analyze()

		// REQ-COD-006: Emit detected=0 for containers in warmup
		detected := 0.0
		if result.Detected {
			detected = 1.0
		}

		// REQ-COD-003: Emit metrics with container tags
		sender.Gauge("container.cpu.oscillation.detected", detected, "", tags)
		sender.Gauge("container.cpu.oscillation.amplitude", result.Amplitude, "", tags)
		sender.Gauge("container.cpu.oscillation.frequency", result.Frequency, "", tags)
		sender.Gauge("container.cpu.oscillation.direction_reversals", float64(result.DirectionReversals), "", tags)
		sender.Gauge("container.cpu.oscillation.baseline_stddev", cd.detector.BaselineStdDev(), "", tags)

		// Emit amplitude_ratio: amplitude / baseline_stddev (raw ratio without multiplier)
		// This allows querying "how many containers have ratio > X?" for any threshold X
		// without needing to redeploy with different amplitude_multiplier config
		amplitudeRatio := 0.0
		if cd.detector.BaselineStdDev() > 0 {
			amplitudeRatio = result.Amplitude / cd.detector.BaselineStdDev()
		}
		sender.Gauge("container.cpu.oscillation.amplitude_ratio", amplitudeRatio, "", tags)

		// Log when oscillation is detected
		if result.Detected {
			shortID := containerID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			log.Infof("[%s] Oscillation detected for container %s: amplitude=%.2f%%, frequency=%.3fHz, direction_reversals=%d",
				CheckName, shortID, result.Amplitude, result.Frequency, result.DirectionReversals)
		}
	}

	// Note: sender.Commit() is called by LongRunningCheckWrapper every 15 seconds
}

// Stop stops the check
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0 to indicate this is a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}
