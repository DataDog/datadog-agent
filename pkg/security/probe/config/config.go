// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	rsNS = "runtime_security_config"
	evNS = "event_monitoring_config"
)

func setEnv() {
	if env.IsContainerized() && filesystem.FileExists("/host") {
		if v := os.Getenv("HOST_PROC"); v == "" {
			os.Setenv("HOST_PROC", "/host/proc")
		}
		if v := os.Getenv("HOST_SYS"); v == "" {
			os.Setenv("HOST_SYS", "/host/sys")
		}
	}
}

// Config defines a security config
type Config struct {
	// event monitor/probe parameters
	ebpf.Config

	// EnableAllProbes defines if all probes should be activated regardless of loaded rules (while still respecting config, especially network disabled)
	EnableAllProbes bool

	// EnableKernelFilters defines if in-kernel filtering should be activated or not
	EnableKernelFilters bool

	// EnableApprovers defines if in-kernel approvers should be activated or not
	EnableApprovers bool

	// EnableDiscarders defines if in-kernel discarders should be activated or not
	EnableDiscarders bool

	// FlushDiscarderWindow defines the maximum time window for discarders removal.
	// This is used during reload to avoid removing all the discarders at the same time.
	FlushDiscarderWindow int

	// SocketPath is the path to the socket that is used to communicate with the security agent and process agent
	SocketPath string

	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int

	// PIDCacheSize is the size of the user space PID caches
	PIDCacheSize int

	// StatsTagsCardinality determines the cardinality level of the tags added to the exported metrics
	StatsTagsCardinality string

	// CustomSensitiveWords defines words to add to the scrubber
	CustomSensitiveWords []string

	// ERPCDentryResolutionEnabled determines if the ERPC dentry resolution is enabled
	ERPCDentryResolutionEnabled bool

	// MapDentryResolutionEnabled determines if the map resolution is enabled
	MapDentryResolutionEnabled bool

	// DentryCacheSize is the size of the user space dentry cache
	DentryCacheSize int

	// NOTE(safchain) need to revisit this one as it can impact multiple event consumers
	// EnvsWithValue lists environnement variables that will be fully exported
	EnvsWithValue []string

	// RuntimeMonitor defines if the Go runtime and system monitor should be enabled
	RuntimeMonitor bool

	// EventStreamUseRingBuffer specifies whether to use eBPF ring buffers when available
	EventStreamUseRingBuffer bool

	// EventStreamBufferSize specifies the buffer size of the eBPF map used for events
	EventStreamBufferSize int

	// EventStreamUseFentry specifies whether to use eBPF fentry when available instead of kprobes
	EventStreamUseFentry bool

	// EventStreamUseKprobeFallback specifies whether to use fentry fallback can be used
	EventStreamUseKprobeFallback bool

	// EventStreamKretprobeMaxActive specifies the maximum number of active kretprobe at a given time
	EventStreamKretprobeMaxActive int

	// RuntimeCompilationEnabled defines if the runtime-compilation is enabled
	RuntimeCompilationEnabled bool

	// NetworkLazyInterfacePrefixes is the list of interfaces prefix that aren't explicitly deleted by the container
	// runtime, and that are lazily deleted by the kernel when a network namespace is cleaned up. This list helps the
	// agent detect when a network namespace should be purged from all caches.
	NetworkLazyInterfacePrefixes []string

	// NetworkClassifierPriority defines the priority at which CWS should insert its TC classifiers.
	NetworkClassifierPriority uint16

	// NetworkClassifierHandle defines the handle at which CWS should insert its TC classifiers.
	NetworkClassifierHandle uint16

	// RawNetworkClassifierHandle defines the handle at which CWS should insert its Raw TC classifiers.
	RawNetworkClassifierHandle uint16

	// NetworkFlowMonitorEnabled defines if the network flow monitor should be enabled.
	NetworkFlowMonitorEnabled bool

	// NetworkFlowMonitorPeriod defines the period at which collected flows should flushed to user space.
	NetworkFlowMonitorPeriod time.Duration

	// NetworkFlowMonitorSKStorageEnabled defines if the network flow monitor should use a SK_STORAGE map (higher memory footprint).
	NetworkFlowMonitorSKStorageEnabled bool

	// ProcessConsumerEnabled defines if the process-agent wants to receive kernel events
	ProcessConsumerEnabled bool

	// NetworkConsumerEnabled defines if the network tracer system-probe module wants to receive kernel events
	NetworkConsumerEnabled bool

	// NetworkEnabled defines if the network probes should be activated
	NetworkEnabled bool

	// NetworkIngressEnabled defines if the network ingress probes should be activated
	NetworkIngressEnabled bool

	// NetworkRawPacketEnabled defines if the network raw packet is enabled
	NetworkRawPacketEnabled bool

	// NetworkRawPacketLimiterRate defines the rate at which raw packets should be sent to user space
	NetworkRawPacketLimiterRate int

	// NetworkRawPacketRestriction defines the global raw packet filter
	NetworkRawPacketFilter string

	// NetworkPrivateIPRanges defines the list of IP that should be considered private
	NetworkPrivateIPRanges []string

	// NetworkExtraPrivateIPRanges defines the list of extra IP that should be considered private
	NetworkExtraPrivateIPRanges []string

	// StatsPollingInterval determines how often metrics should be polled
	StatsPollingInterval time.Duration

	// SyscallsMonitorEnabled defines if syscalls monitoring metrics should be collected
	SyscallsMonitorEnabled bool

	// DNSResolverCacheSize is the numer of entries in the DNS resolver LRU cache
	DNSResolverCacheSize int

	// DNSResolutionEnabled resolving DNS names from IP addresses
	DNSResolutionEnabled bool

	// SpanTrackingEnabled defines if span tracking should be enabled
	SpanTrackingEnabled bool

	// SpanTrackingCacheSize is the size of the span tracking cache
	SpanTrackingCacheSize int
}

// NewConfig returns a new Config object
func NewConfig() (*Config, error) {
	sysconfig.Adjust(pkgconfigsetup.SystemProbe())

	setEnv()

	c := &Config{
		Config:                             *ebpf.NewConfig(),
		EnableAllProbes:                    getBool("enable_all_probes"),
		EnableKernelFilters:                getBool("enable_kernel_filters"),
		EnableApprovers:                    getBool("enable_approvers"),
		EnableDiscarders:                   getBool("enable_discarders"),
		FlushDiscarderWindow:               getInt("flush_discarder_window"),
		PIDCacheSize:                       getInt("pid_cache_size"),
		StatsTagsCardinality:               getString("events_stats.tags_cardinality"),
		CustomSensitiveWords:               getStringSlice("custom_sensitive_words"),
		ERPCDentryResolutionEnabled:        getBool("erpc_dentry_resolution_enabled"),
		MapDentryResolutionEnabled:         getBool("map_dentry_resolution_enabled"),
		DentryCacheSize:                    getInt("dentry_cache_size"),
		RuntimeMonitor:                     getBool("runtime_monitor.enabled"),
		NetworkLazyInterfacePrefixes:       getStringSlice("network.lazy_interface_prefixes"),
		NetworkClassifierPriority:          uint16(getInt("network.classifier_priority")),
		NetworkClassifierHandle:            uint16(getInt("network.classifier_handle")),
		RawNetworkClassifierHandle:         uint16(getInt("network.raw_classifier_handle")),
		NetworkFlowMonitorPeriod:           getDuration("network.flow_monitor.period"),
		NetworkFlowMonitorEnabled:          getBool("network.flow_monitor.enabled"),
		NetworkFlowMonitorSKStorageEnabled: getBool("network.flow_monitor.sk_storage.enabled"),
		EventStreamUseRingBuffer:           getBool("event_stream.use_ring_buffer"),
		EventStreamBufferSize:              getInt("event_stream.buffer_size"),
		EventStreamUseFentry:               getBool("event_stream.use_fentry"),
		EventStreamUseKprobeFallback:       getBool("event_stream.use_kprobe_fallback"),
		EventStreamKretprobeMaxActive:      getInt("event_stream.kretprobe_max_active"),

		EnvsWithValue:               getStringSlice("envs_with_value"),
		NetworkEnabled:              getBool("network.enabled"),
		NetworkIngressEnabled:       getBool("network.ingress.enabled"),
		NetworkRawPacketEnabled:     getBool("network.raw_packet.enabled"),
		NetworkRawPacketLimiterRate: getInt("network.raw_packet.limiter_rate"),
		NetworkRawPacketFilter:      getString("network.raw_packet.filter"),
		NetworkPrivateIPRanges:      getStringSlice("network.private_ip_ranges"),
		NetworkExtraPrivateIPRanges: getStringSlice("network.extra_private_ip_ranges"),
		StatsPollingInterval:        time.Duration(getInt("events_stats.polling_interval")) * time.Second,
		SyscallsMonitorEnabled:      getBool("syscalls_monitor.enabled"),
		DNSResolverCacheSize:        getInt("dns_resolution.cache_size"),
		DNSResolutionEnabled:        getBool("dns_resolution.enabled"),

		// event server
		SocketPath:       pkgconfigsetup.SystemProbe().GetString(join(evNS, "socket")),
		EventServerBurst: pkgconfigsetup.SystemProbe().GetInt(join(evNS, "event_server.burst")),

		// runtime compilation
		RuntimeCompilationEnabled: getBool("runtime_compilation.enabled"),

		// span tracking
		SpanTrackingEnabled:   getBool("span_tracking.enabled"),
		SpanTrackingCacheSize: getInt("span_tracking.cache_size"),
	}

	if err := c.sanitize(); err != nil {
		return nil, err
	}

	return c, nil
}

// sanitize config parameters
func (c *Config) sanitize() error {
	if !c.ERPCDentryResolutionEnabled && !c.MapDentryResolutionEnabled {
		c.MapDentryResolutionEnabled = true
	}

	if c.NetworkRawPacketEnabled {
		if c.RawNetworkClassifierHandle != c.NetworkClassifierHandle {
			if c.NetworkClassifierHandle*c.RawNetworkClassifierHandle == 0 {
				return fmt.Errorf("none or both of network.classifier_handle and network.raw_classifier_handle must be provided: got classifier_handle:%d raw_classifier_handle:%d", c.NetworkClassifierHandle, c.RawNetworkClassifierHandle)
			}
		} else {
			if c.NetworkClassifierHandle*c.RawNetworkClassifierHandle != 0 {
				return fmt.Errorf("network.classifier_handle and network.raw_classifier_handle can't be equal and not null: got classifier_handle:%d raw_classifier_handle:%d", c.NetworkClassifierHandle, c.RawNetworkClassifierHandle)
			}
		}
	}

	// not enable at the system-probe level, disable for cws as well
	if !c.Config.EnableRuntimeCompiler {
		c.RuntimeCompilationEnabled = false
	}

	if c.EventStreamBufferSize%os.Getpagesize() != 0 || c.EventStreamBufferSize&(c.EventStreamBufferSize-1) != 0 {
		return fmt.Errorf("runtime_security_config.event_stream.buffer_size must be a power of 2 and a multiple of %d", os.Getpagesize())
	}

	if !isConfigured("enable_approvers") && c.EnableKernelFilters {
		c.EnableApprovers = true
	}

	if !isConfigured("enable_discarders") && c.EnableKernelFilters {
		c.EnableDiscarders = true
	}

	if !c.EnableApprovers && !c.EnableDiscarders {
		c.EnableKernelFilters = false
	}

	c.sanitizeConfigNetwork()

	return nil
}

// sanitizeNetworkConfiguration ensures that event_monitoring_config.network is properly configured
func (c *Config) sanitizeConfigNetwork() {
	lazyInterfaces := make(map[string]bool)
	for _, name := range c.NetworkLazyInterfacePrefixes {
		lazyInterfaces[name] = true
	}
	// make sure to append both `lo` and `dummy` in the list of `event_monitoring_config.network.lazy_interface_prefixes`
	lazyDefaults := []string{"lo", "dummy"}
	for _, name := range lazyDefaults {
		if !lazyInterfaces[name] {
			c.NetworkLazyInterfacePrefixes = append(c.NetworkLazyInterfacePrefixes, name)
		}
	}
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

func getAllKeys(key string) (string, string) {
	deprecatedKey := strings.Join([]string{rsNS, key}, ".")
	newKey := strings.Join([]string{evNS, key}, ".")
	return deprecatedKey, newKey
}

func isConfigured(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	return pkgconfigsetup.SystemProbe().IsConfigured(deprecatedKey) || pkgconfigsetup.SystemProbe().IsConfigured(newKey)
}

func getBool(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsConfigured(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetBool(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetBool(newKey)
}

func getInt(key string) int {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsConfigured(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetInt(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetInt(newKey)
}

func getDuration(key string) time.Duration {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetDuration(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetDuration(newKey)
}

func getString(key string) string {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsConfigured(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetString(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetString(newKey)
}

func getStringSlice(key string) []string {
	deprecatedKey, newKey := getAllKeys(key)
	if pkgconfigsetup.SystemProbe().IsConfigured(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return pkgconfigsetup.SystemProbe().GetStringSlice(deprecatedKey)
	}
	return pkgconfigsetup.SystemProbe().GetStringSlice(newKey)
}
