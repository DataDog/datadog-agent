// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package config provides the GPU monitoring config.
package config

import (
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	// DisabledCollectors lists Agent GPU collectors that should not be created.
	DisabledCollectors []string
	// NVLinkFECLightErrorThreshold is the maximum corrected-error count classified as light.
	NVLinkFECLightErrorThreshold int
	// LegacySMActive indicates whether the legacy sm_active metric should be emitted.
	LegacySMActive bool
	// StaticMetricsReportingInterval is the reporting interval for static GPU metrics.
	StaticMetricsReportingInterval time.Duration
	// Enabled indicates whether the GPU monitoring probe is enabled.
	Enabled bool
	// EnableEBPFProbes indicates whether the GPU monitoring eBPF probes should be loaded.
	EnableEBPFProbes bool
	// DriverEventsEnabled indicates whether NVIDIA driver events should be collected from the kernel log.
	DriverEventsEnabled bool
	// PRMEndpointEnabled indicates whether the privileged PRM endpoint should be exposed.
	PRMEndpointEnabled bool
	// ScanProcessesInterval is the interval at which the probe scans for new or terminated processes.
	ScanProcessesInterval time.Duration
	// InitialProcessSync indicates whether the probe should sync the process list on startup.
	InitialProcessSync bool
	// ConfigureCgroupPerms indicates whether the probe should configure cgroup permissions for GPU monitoring
	ConfigureCgroupPerms bool
	// EnableFatbinParsing indicates whether the probe should enable fatbin parsing.
	EnableFatbinParsing bool
	// KernelCacheQueueSize is the size of the kernel cache queue for parsing requests
	KernelCacheQueueSize int
	// RingBufferSizePagesPerDevice is the number of pages to use for the ring buffer per device.
	RingBufferSizePagesPerDevice int
	// RingBufferWakeupSize is the number of bytes that need to be available in the ring buffer before waking up userspace.
	RingBufferWakeupSize int
	// RingBufferFlushInterval is the interval at which the ring buffer should be flushed
	RingBufferFlushInterval time.Duration
	// StreamConfig is the configuration for the streams.
	StreamConfig StreamConfig
	// AttacherDetailedLogs indicates whether the probe should enable detailed logs for the uprobe attacher.
	AttacherDetailedLogs bool
	// DeviceCacheRefreshInterval is the interval at which the probe scans for the latest devices
	DeviceCacheRefreshInterval time.Duration
	// CgroupReapplyInterval is the interval at which to re-apply cgroup device configuration. 0 means no re-application.
	// Defaults to 30 seconds. It is used to fix race conditions between systemd and the system-probe permission patching.
	CgroupReapplyInterval time.Duration
	// CgroupReapplyInfinitely controls whether the cgroup device configuration should be reapplied infinitely (true) or only once (false).
	// Defaults to false. When true, the configuration will be reapplied every CgroupReapplyInterval interval.
	CgroupReapplyInfinitely bool
	// JobsConfig provides the ability for the user to define run/group identifiers to be attached to traces and metrics.
	JobsConfig JobsConfig
}

// StreamConfig is the configuration for the streams.
type StreamConfig struct {
	// MaxActiveStreams is the maximum number of streams that can be processed concurrently.
	MaxActiveStreams int
	// Timeout is the maximum time to wait for a stream to be inactive before flushing it.
	Timeout time.Duration
	// MaxKernelLaunches is the maximum number of kernel launches to process per stream before forcing a sync.
	MaxKernelLaunches int
	// MaxMemAllocEvents is the maximum number of memory allocation events to process per stream before evicting the oldest events.
	MaxMemAllocEvents int
	// MaxPendingKernelSpans is the maximum number of pending kernel spans to keep in each stream handler.
	MaxPendingKernelSpans int
	// MaxPendingMemorySpans is the maximum number of pending memory allocation spans to keep in each stream handler.
	MaxPendingMemorySpans int
}

// JobsConfig lets users define which pod label or annotation identifies a training job.
type JobsConfig struct {
	// Run is a unique id for a training run.
	Run IdentifierConfig
	// Group is an id for a group of training runs.
	Group IdentifierConfig
}

// IdentifierType is the kind of metadata an identifier is read from.
type IdentifierType string

const (
	// IdentifierTypeLabel means the identifier is read from a pod label.
	IdentifierTypeLabel IdentifierType = "label"
	// IdentifierTypeAnnotation means the identifier is read from a pod annotation.
	IdentifierTypeAnnotation IdentifierType = "annotation"
	// IdentifierTypeEnv means the identifier is read from an environment variable of the container.
	IdentifierTypeEnv IdentifierType = "env"
)

// IdentifierConfig points at a pod label, pod annotation or container environment variable holding an identifier.
type IdentifierConfig struct {
	// Key is the name of the label, annotation or environment variable. Empty means not configured.
	Key string
	// Type is whether Key is a label, an annotation or an environment variable.
	Type IdentifierType
}

// Configured returns true if the identifier has a key and a supported type.
func (i IdentifierConfig) Configured() bool {
	if i.Key == "" {
		return false
	}
	switch i.Type {
	case IdentifierTypeLabel, IdentifierTypeAnnotation, IdentifierTypeEnv:
		return true
	default:
		return false
	}
}

// UsesPodMetadata returns true if the identifier is read from pod labels or annotations.
func (i IdentifierConfig) UsesPodMetadata() bool {
	return i.Type == IdentifierTypeLabel || i.Type == IdentifierTypeAnnotation
}

// EnvKeys returns the names of the environment variables the identifiers are read from.
func (j JobsConfig) EnvKeys() []string {
	var keys []string
	for _, id := range []IdentifierConfig{j.Run, j.Group} {
		if id.Configured() && id.Type == IdentifierTypeEnv && !slices.Contains(keys, id.Key) {
			keys = append(keys, id.Key)
		}
	}
	return keys
}

// NewJobsConfig reads the training job identifiers from the agent configuration.
func NewJobsConfig() JobsConfig {
	agentCfg := pkgconfigsetup.Datadog()
	return JobsConfig{
		Run:   newIdentifierConfig(agentCfg, "gpu.jobs.run"),
		Group: newIdentifierConfig(agentCfg, "gpu.jobs.group"),
	}
}

func newIdentifierConfig(cfg model.Reader, prefix string) IdentifierConfig {
	return IdentifierConfig{
		Key:  cfg.GetString(prefix + ".key"),
		Type: IdentifierType(strings.ToLower(cfg.GetString(prefix + ".type"))),
	}
}

// New generates a new configuration for the GPU monitoring probe.
func New() *Config {
	spCfg := pkgconfigsetup.SystemProbe()
	agentCfg := pkgconfigsetup.Datadog()
	return &Config{
		DisabledCollectors:             agentCfg.GetStringSlice("gpu.disabled_collectors"),
		NVLinkFECLightErrorThreshold:   agentCfg.GetInt("gpu.nvlink.fec_light_error_threshold"),
		LegacySMActive:                 agentCfg.GetBool("gpu.legacy_sm_active"),
		StaticMetricsReportingInterval: agentCfg.GetDuration("gpu.static_metrics_reporting_interval"),
		ScanProcessesInterval:          time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "process_scan_interval_seconds"))) * time.Second,
		InitialProcessSync:             spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "initial_process_sync")),
		Enabled:                        spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enabled")),
		EnableEBPFProbes:               spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enable_ebpf_probes")),
		DriverEventsEnabled:            spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "driver_events_enabled")),
		PRMEndpointEnabled:             spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "prm_endpoint_enabled")),
		ConfigureCgroupPerms:           spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "configure_cgroup_perms")),
		EnableFatbinParsing:            spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enable_fatbin_parsing")),
		KernelCacheQueueSize:           spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "fatbin_request_queue_size")),
		RingBufferSizePagesPerDevice:   spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "ring_buffer_pages_per_device")),
		RingBufferWakeupSize:           spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "ringbuffer_wakeup_size")),
		RingBufferFlushInterval:        spCfg.GetDuration(sysconfig.FullKeyPath(consts.GPUNS, "ringbuffer_flush_interval")),
		StreamConfig: StreamConfig{
			MaxActiveStreams:      spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "max_active")),
			Timeout:               time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "timeout_seconds"))) * time.Second,
			MaxKernelLaunches:     spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "max_kernel_launches")),
			MaxMemAllocEvents:     spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "max_mem_alloc_events")),
			MaxPendingKernelSpans: spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "max_pending_kernel_spans")),
			MaxPendingMemorySpans: spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "streams", "max_pending_memory_spans")),
		},
		AttacherDetailedLogs:       spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "attacher_detailed_logs")),
		DeviceCacheRefreshInterval: spCfg.GetDuration(sysconfig.FullKeyPath(consts.GPUNS, "device_cache_refresh_interval")),
		CgroupReapplyInterval:      spCfg.GetDuration(sysconfig.FullKeyPath(consts.GPUNS, "cgroup_reapply_interval")),
		CgroupReapplyInfinitely:    spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "cgroup_reapply_infinitely")),
		JobsConfig:                 NewJobsConfig(),
	}
}
