// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// Config holds the configuration for the runtime security agent
type Config struct {
	ebpf.Config

	// RuntimeEnabled defines if the runtime security module should be enabled
	RuntimeEnabled bool
	// PoliciesDir defines the folder in which the policy files are located
	PoliciesDir string
	// EnableKernelFilters defines if in-kernel filtering should be activated or not
	EnableKernelFilters bool
	// EnableApprovers defines if in-kernel approvers should be activated or not
	EnableApprovers bool
	// EnableDiscarders defines if in-kernel discarders should be activated or not
	EnableDiscarders bool
	// FlushDiscarderWindow defines the maximum time window for discarders removal.
	// This is used during reload to avoid removing all the discarders at the same time.
	FlushDiscarderWindow int
	// SocketPath is the path to the socket that is used to communicate with the security agent
	SocketPath string
	// SyscallMonitor defines if the syscall monitor should be activated or not
	SyscallMonitor bool
	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int
	// EventServerRate defines the grpc server rate at which events can be sent
	EventServerRate int
	// EventServerRetention defines an event retention period so that some fields can be resolved
	EventServerRetention int
	// PIDCacheSize is the size of the user space PID caches
	PIDCacheSize int
	// CookieCacheSize is the size of the cookie cache used to cache process context
	CookieCacheSize int
	// LoadControllerEventsCountThreshold defines the amount of events past which we will trigger the in-kernel circuit breaker
	LoadControllerEventsCountThreshold int64
	// LoadControllerDiscarderTimeout defines the amount of time discarders set by the load controller should last
	LoadControllerDiscarderTimeout time.Duration
	// LoadControllerControlPeriod defines the period at which the load controller will empty the user space counter used
	// to evaluate the amount of events brought back to user space
	LoadControllerControlPeriod time.Duration
	// StatsPollingInterval determines how often metrics should be polled
	StatsPollingInterval time.Duration
	// StatsTagsCardinality determines the cardinality level of the tags added to the exported metrics
	StatsTagsCardinality string
	// StatsdAddr defines the statsd address
	StatsdAddr string
	// AgentMonitoringEvents determines if the monitoring events of the agent should be sent to Datadog
	AgentMonitoringEvents bool
	// FIMEnabled determines whether fim rules will be loaded
	FIMEnabled bool
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
	// HostServiceName string
	HostServiceName string
	// LogPatterns pattern to be used by the logger for trace level
	LogPatterns []string
	// LogTags tags to be used by the logger for trace level
	LogTags []string
	// SelfTestEnabled defines if the self tester should be enabled (useful for tests for example)
	SelfTestEnabled bool
	// EnableRemoteConfig defines if the agent configuration should be fetched from the backend
	EnableRemoteConfig bool
	// ActivityDumpEnabled defines if the activity dump manager should be enabled
	ActivityDumpEnabled bool
	// ActivityDumpCleanupPeriod defines the period at which the activity dump manager should perform its cleanup
	// operation.
	ActivityDumpCleanupPeriod time.Duration
	// ActivityDumpTagsResolutionPeriod defines the period at which the activity dump manager should try to resolve
	// missing container tags.
	ActivityDumpTagsResolutionPeriod time.Duration
	// ActivityDumpTracedCgroupsCount defines the maximum count of cgroups that should be monitored concurrently. Set
	// this parameter to -1 to monitor all cgroups at the same time. Leave this parameter to 0 to prevent the generation
	// of activity dumps based on cgroups.
	ActivityDumpTracedCgroupsCount int
	// ActivityDumpTracedEventTypes defines the list of events that should be captured in an activity dump. Leave this
	// parameter empty to monitor all event types. If not already present, the `exec` event will automatically be added
	// to this list.
	ActivityDumpTracedEventTypes []model.EventType
	// ActivityDumpCgroupDumpTimeout defines the cgroup activity dumps timeout.
	ActivityDumpCgroupDumpTimeout time.Duration
	// ActivityDumpCgroupWaitListSize defines the size of the cgroup wait list. The wait list is used to introduce a
	// delay between 2 activity dumps of the same cgroup.
	ActivityDumpCgroupWaitListSize int
	// ActivityDumpCgroupOutputDirectory defines the output directory for the cgroup activity dumps and graphs. Leave
	// this field empty to prevent writing any output to disk.
	ActivityDumpCgroupOutputDirectory string
	// RuntimeMonitor defines if the runtime monitor should be enabled
	RuntimeMonitor bool
	// NetworkEnabled defines if the network probes should be activated
	NetworkEnabled bool
	// NetworkLazyInterfacePrefixes is the list of interfaces prefix that aren't explicitly deleted by the container
	// runtime, and that are lazily deleted by the kernel when a network namespace is cleaned up. This list helps the
	// agent detect when a network namespace should be purged from all caches.
	NetworkLazyInterfacePrefixes []string
	// RuntimeCompilationEnabled defines if the runtime-compilation is enabled
	RuntimeCompilationEnabled bool
	// EnableRuntimeCompiledConstants defines if the runtime compilation based constant fetcher is enabled
	RuntimeCompiledConstantsEnabled bool
	// RuntimeCompiledConstantsIsSet is set if the runtime compiled constants option is user-set
	RuntimeCompiledConstantsIsSet bool
	// EventMonitoring enabled event monitoring
	EventMonitoring bool
}

// IsEnabled returns true if any feature is enabled. Has to be applied in config package too
func (c *Config) IsEnabled() bool {
	return c.RuntimeEnabled || c.FIMEnabled
}

func setEnv() {
	if aconfig.IsContainerized() && util.PathExists("/host") {
		if v := os.Getenv("HOST_PROC"); v == "" {
			os.Setenv("HOST_PROC", "/host/proc")
		}
		if v := os.Getenv("HOST_SYS"); v == "" {
			os.Setenv("HOST_SYS", "/host/sys")
		}
	}
}

// NewConfig returns a new Config object
func NewConfig(cfg *config.Config) (*Config, error) {
	c := &Config{
		Config:                             *ebpf.NewConfig(),
		RuntimeEnabled:                     aconfig.Datadog.GetBool("runtime_security_config.enabled"),
		FIMEnabled:                         aconfig.Datadog.GetBool("runtime_security_config.fim_enabled"),
		EventMonitoring:                    aconfig.Datadog.GetBool("runtime_security_config.event_monitoring.enabled"),
		EnableKernelFilters:                aconfig.Datadog.GetBool("runtime_security_config.enable_kernel_filters"),
		EnableApprovers:                    aconfig.Datadog.GetBool("runtime_security_config.enable_approvers"),
		EnableDiscarders:                   aconfig.Datadog.GetBool("runtime_security_config.enable_discarders"),
		FlushDiscarderWindow:               aconfig.Datadog.GetInt("runtime_security_config.flush_discarder_window"),
		SocketPath:                         aconfig.Datadog.GetString("runtime_security_config.socket"),
		SyscallMonitor:                     aconfig.Datadog.GetBool("runtime_security_config.syscall_monitor.enabled"),
		PoliciesDir:                        aconfig.Datadog.GetString("runtime_security_config.policies.dir"),
		EventServerBurst:                   aconfig.Datadog.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:                    aconfig.Datadog.GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention:               aconfig.Datadog.GetInt("runtime_security_config.event_server.retention"),
		PIDCacheSize:                       aconfig.Datadog.GetInt("runtime_security_config.pid_cache_size"),
		CookieCacheSize:                    aconfig.Datadog.GetInt("runtime_security_config.cookie_cache_size"),
		LoadControllerEventsCountThreshold: int64(aconfig.Datadog.GetInt("runtime_security_config.load_controller.events_count_threshold")),
		LoadControllerDiscarderTimeout:     time.Duration(aconfig.Datadog.GetInt("runtime_security_config.load_controller.discarder_timeout")) * time.Second,
		LoadControllerControlPeriod:        time.Duration(aconfig.Datadog.GetInt("runtime_security_config.load_controller.control_period")) * time.Second,
		StatsPollingInterval:               time.Duration(aconfig.Datadog.GetInt("runtime_security_config.events_stats.polling_interval")) * time.Second,
		StatsTagsCardinality:               aconfig.Datadog.GetString("runtime_security_config.events_stats.tags_cardinality"),
		StatsdAddr:                         fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort),
		AgentMonitoringEvents:              aconfig.Datadog.GetBool("runtime_security_config.agent_monitoring_events"),
		CustomSensitiveWords:               aconfig.Datadog.GetStringSlice("runtime_security_config.custom_sensitive_words"),
		ERPCDentryResolutionEnabled:        aconfig.Datadog.GetBool("runtime_security_config.erpc_dentry_resolution_enabled"),
		MapDentryResolutionEnabled:         aconfig.Datadog.GetBool("runtime_security_config.map_dentry_resolution_enabled"),
		DentryCacheSize:                    aconfig.Datadog.GetInt("runtime_security_config.dentry_cache_size"),
		RemoteTaggerEnabled:                aconfig.Datadog.GetBool("runtime_security_config.remote_tagger"),
		LogPatterns:                        aconfig.Datadog.GetStringSlice("runtime_security_config.log_patterns"),
		LogTags:                            aconfig.Datadog.GetStringSlice("runtime_security_config.log_tags"),
		SelfTestEnabled:                    aconfig.Datadog.GetBool("runtime_security_config.self_test.enabled"),
		EnableRemoteConfig:                 aconfig.Datadog.GetBool("runtime_security_config.enable_remote_configuration"),
		ActivityDumpEnabled:                aconfig.Datadog.GetBool("runtime_security_config.activity_dump.enabled"),
		ActivityDumpCleanupPeriod:          time.Duration(aconfig.Datadog.GetInt("runtime_security_config.activity_dump.cleanup_period")) * time.Second,
		ActivityDumpTagsResolutionPeriod:   time.Duration(aconfig.Datadog.GetInt("runtime_security_config.activity_dump.tags_resolution_period")) * time.Second,
		ActivityDumpTracedCgroupsCount:     aconfig.Datadog.GetInt("runtime_security_config.activity_dump.traced_cgroups_count"),
		ActivityDumpTracedEventTypes:       model.ParseEventTypeStringSlice(aconfig.Datadog.GetStringSlice("runtime_security_config.activity_dump.traced_event_types")),
		ActivityDumpCgroupDumpTimeout:      time.Duration(aconfig.Datadog.GetInt("runtime_security_config.activity_dump.cgroup_dump_timeout")) * time.Minute,
		ActivityDumpCgroupWaitListSize:     aconfig.Datadog.GetInt("runtime_security_config.activity_dump.cgroup_wait_list_size"),
		ActivityDumpCgroupOutputDirectory:  aconfig.Datadog.GetString("runtime_security_config.activity_dump.cgroup_output_directory"),
		RuntimeMonitor:                     aconfig.Datadog.GetBool("runtime_security_config.runtime_monitor.enabled"),
		NetworkEnabled:                     aconfig.Datadog.GetBool("runtime_security_config.network.enabled"),
		NetworkLazyInterfacePrefixes:       aconfig.Datadog.GetStringSlice("runtime_security_config.network.lazy_interface_prefixes"),
		// runtime compilation
		RuntimeCompilationEnabled:       aconfig.Datadog.GetBool("runtime_security_config.runtime_compilation.enabled"),
		RuntimeCompiledConstantsEnabled: aconfig.Datadog.GetBool("runtime_security_config.runtime_compilation.compiled_constants_enabled"),
		RuntimeCompiledConstantsIsSet:   aconfig.Datadog.IsSet("runtime_security_config.runtime_compilation.compiled_constants_enabled"),
	}

	// if runtime is enabled then we force fim
	if c.RuntimeEnabled {
		c.FIMEnabled = true
	}

	if !c.IsEnabled() {
		return c, nil
	}

	if !aconfig.Datadog.IsSet("runtime_security_config.enable_approvers") && c.EnableKernelFilters {
		c.EnableApprovers = true
	}

	if !aconfig.Datadog.IsSet("runtime_security_config.enable_discarders") && c.EnableKernelFilters {
		c.EnableDiscarders = true
	}

	if !c.EnableApprovers && !c.EnableDiscarders {
		c.EnableKernelFilters = false
	}

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

	serviceName := utils.GetTagValue("service", aconfig.GetConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}

	var found bool
	for _, evtType := range c.ActivityDumpTracedEventTypes {
		if evtType == model.ExecEventType {
			found = true
		}
	}
	if !found {
		c.ActivityDumpTracedEventTypes = append(c.ActivityDumpTracedEventTypes, model.ExecEventType)
	}

	lazyInterfaces := make(map[string]bool)
	for _, name := range c.NetworkLazyInterfacePrefixes {
		lazyInterfaces[name] = true
	}
	// make sure to append both `lo` and `dummy` in the list of `runtime_security_config.network.lazy_interface_prefixes`
	lazyDefaults := []string{"lo", "dummy"}
	for _, name := range lazyDefaults {
		if !lazyInterfaces[name] {
			c.NetworkLazyInterfacePrefixes = append(c.NetworkLazyInterfacePrefixes, name)
		}
	}

	setEnv()
	return c, nil
}
