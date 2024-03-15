// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config holds config related files
package config

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	rsNS = "runtime_security_config"
	evNS = "event_monitoring_config"
)

func setEnv() {
	if coreconfig.IsContainerized() && filesystem.FileExists("/host") {
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

	// RemoteTaggerEnabled defines whether the remote tagger is enabled
	RemoteTaggerEnabled bool

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

	// RuntimeCompilationEnabled defines if the runtime-compilation is enabled
	RuntimeCompilationEnabled bool

	// EnableRuntimeCompiledConstants defines if the runtime compilation based constant fetcher is enabled
	RuntimeCompiledConstantsEnabled bool

	// RuntimeCompiledConstantsIsSet is set if the runtime compiled constants option is user-set
	RuntimeCompiledConstantsIsSet bool

	// NetworkLazyInterfacePrefixes is the list of interfaces prefix that aren't explicitly deleted by the container
	// runtime, and that are lazily deleted by the kernel when a network namespace is cleaned up. This list helps the
	// agent detect when a network namespace should be purged from all caches.
	NetworkLazyInterfacePrefixes []string

	// NetworkClassifierPriority defines the priority at which CWS should insert its TC classifiers.
	NetworkClassifierPriority uint16

	// NetworkClassifierHandle defines the handle at which CWS should insert its TC classifiers.
	NetworkClassifierHandle uint16

	// ProcessConsumerEnabled defines if the process-agent wants to receive kernel events
	ProcessConsumerEnabled bool

	// NetworkConsumerEnabled defines if the network tracer system-probe module wants to receive kernel events
	NetworkConsumerEnabled bool

	// NetworkEnabled defines if the network probes should be activated
	NetworkEnabled bool

	// StatsPollingInterval determines how often metrics should be polled
	StatsPollingInterval time.Duration

	// SyscallsMonitorEnabled defines if syscalls monitoring metrics should be collected
	SyscallsMonitorEnabled bool
}

// NewConfig returns a new Config object
func NewConfig() (*Config, error) {
	sysconfig.Adjust(coreconfig.SystemProbe)

	setEnv()

	c := &Config{
		Config:                       *ebpf.NewConfig(),
		EnableAllProbes:              getBool("enable_all_probes"),
		EnableKernelFilters:          getBool("enable_kernel_filters"),
		EnableApprovers:              getBool("enable_approvers"),
		EnableDiscarders:             getBool("enable_discarders"),
		FlushDiscarderWindow:         getInt("flush_discarder_window"),
		PIDCacheSize:                 getInt("pid_cache_size"),
		StatsTagsCardinality:         getString("events_stats.tags_cardinality"),
		CustomSensitiveWords:         getStringSlice("custom_sensitive_words"),
		ERPCDentryResolutionEnabled:  getBool("erpc_dentry_resolution_enabled"),
		MapDentryResolutionEnabled:   getBool("map_dentry_resolution_enabled"),
		DentryCacheSize:              getInt("dentry_cache_size"),
		RemoteTaggerEnabled:          getBool("remote_tagger"),
		RuntimeMonitor:               getBool("runtime_monitor.enabled"),
		NetworkLazyInterfacePrefixes: getStringSlice("network.lazy_interface_prefixes"),
		NetworkClassifierPriority:    uint16(getInt("network.classifier_priority")),
		NetworkClassifierHandle:      uint16(getInt("network.classifier_handle")),
		EventStreamUseRingBuffer:     getBool("event_stream.use_ring_buffer"),
		EventStreamBufferSize:        getInt("event_stream.buffer_size"),
		EventStreamUseFentry:         getEventStreamFentryValue(),
		EnvsWithValue:                getStringSlice("envs_with_value"),
		NetworkEnabled:               getBool("network.enabled"),
		StatsPollingInterval:         time.Duration(getInt("events_stats.polling_interval")) * time.Second,
		SyscallsMonitorEnabled:       getBool("syscalls_monitor.enabled"),

		// event server
		SocketPath:       coreconfig.SystemProbe.GetString(join(evNS, "socket")),
		EventServerBurst: coreconfig.SystemProbe.GetInt(join(evNS, "event_server.burst")),

		// runtime compilation
		RuntimeCompilationEnabled:       getBool("runtime_compilation.enabled"),
		RuntimeCompiledConstantsEnabled: getBool("runtime_compilation.compiled_constants_enabled"),
		RuntimeCompiledConstantsIsSet:   isSet("runtime_compilation.compiled_constants_enabled"),
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

	// not enable at the system-probe level, disable for cws as well
	if !c.Config.EnableRuntimeCompiler {
		c.RuntimeCompilationEnabled = false
	}

	if !c.RuntimeCompilationEnabled {
		c.RuntimeCompiledConstantsEnabled = false
	}

	if c.EventStreamBufferSize%os.Getpagesize() != 0 || c.EventStreamBufferSize&(c.EventStreamBufferSize-1) != 0 {
		return fmt.Errorf("runtime_security_config.event_stream.buffer_size must be a power of 2 and a multiple of %d", os.Getpagesize())
	}

	if !isSet("enable_approvers") && c.EnableKernelFilters {
		c.EnableApprovers = true
	}

	if !isSet("enable_discarders") && c.EnableKernelFilters {
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

func getEventStreamFentryValue() bool {
	if getBool("event_stream.use_fentry") {
		return true
	}

	switch runtime.GOARCH {
	case "amd64":
		return getBool("event_stream.use_fentry_amd64")
	case "arm64":
		return getBool("event_stream.use_fentry_arm64")
	default:
		return false
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

func isSet(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	return coreconfig.SystemProbe.IsSet(deprecatedKey) || coreconfig.SystemProbe.IsSet(newKey)
}

func getBool(key string) bool {
	deprecatedKey, newKey := getAllKeys(key)
	if coreconfig.SystemProbe.IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return coreconfig.SystemProbe.GetBool(deprecatedKey)
	}
	return coreconfig.SystemProbe.GetBool(newKey)
}

func getInt(key string) int {
	deprecatedKey, newKey := getAllKeys(key)
	if coreconfig.SystemProbe.IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return coreconfig.SystemProbe.GetInt(deprecatedKey)
	}
	return coreconfig.SystemProbe.GetInt(newKey)
}

func getString(key string) string {
	deprecatedKey, newKey := getAllKeys(key)
	if coreconfig.SystemProbe.IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return coreconfig.SystemProbe.GetString(deprecatedKey)
	}
	return coreconfig.SystemProbe.GetString(newKey)
}

func getStringSlice(key string) []string {
	deprecatedKey, newKey := getAllKeys(key)
	if coreconfig.SystemProbe.IsSet(deprecatedKey) {
		log.Warnf("%s has been deprecated: please set %s instead", deprecatedKey, newKey)
		return coreconfig.SystemProbe.GetStringSlice(deprecatedKey)
	}
	return coreconfig.SystemProbe.GetStringSlice(newKey)
}
