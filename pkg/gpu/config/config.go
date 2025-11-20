// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package config provides the GPU monitoring config.
package config

import (
	"errors"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// ErrNotSupported is the error returned if GPU monitoring is not supported on this platform
var ErrNotSupported = errors.New("GPU Monitoring is not supported")

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	ebpf.Config
	// Enabled indicates whether the GPU monitoring probe is enabled.
	Enabled bool
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

// New generates a new configuration for the GPU monitoring probe.
func New() *Config {
	spCfg := pkgconfigsetup.SystemProbe()
	return &Config{
		Config:                       *ebpf.NewConfig(),
		ScanProcessesInterval:        time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "process_scan_interval_seconds"))) * time.Second,
		InitialProcessSync:           spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "initial_process_sync")),
		Enabled:                      spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enabled")),
		ConfigureCgroupPerms:         spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "configure_cgroup_perms")),
		EnableFatbinParsing:          spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enable_fatbin_parsing")),
		KernelCacheQueueSize:         spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "fatbin_request_queue_size")),
		RingBufferSizePagesPerDevice: spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "ring_buffer_pages_per_device")),
		RingBufferWakeupSize:         spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "ringbuffer_wakeup_size")),
		RingBufferFlushInterval:      spCfg.GetDuration(sysconfig.FullKeyPath(consts.GPUNS, "ringbuffer_flush_interval")),
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
	}
}
