// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"time"

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
	// SelfTestEnabled defines if the self tester should be enabled (useful for tests for example)
	SelfTestEnabled bool
	// EnableRemoteConfig defines if configuration should be fetched from the backend
	EnableRemoteConfig bool
	// EnableRuntimeCompiledConstants defines if the runtime compilation based constant fetcher is enabled
	EnableRuntimeCompiledConstants bool
	// RuntimeCompiledConstantsIsSet is set if the runtime compiled constants option is user-set
	RuntimeCompiledConstantsIsSet bool
	// ActivityDumpEnabled defines if the activity dump manager should be enabled
	ActivityDumpEnabled bool
	// ActivityDumpCleanupPeriod defines the period at which the activity dump manager should perform its cleanup
	// operation.
	ActivityDumpCleanupPeriod time.Duration
	// RuntimeMonitor defines if the runtime monitor should be enabled
	RuntimeMonitor bool
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
		SelfTestEnabled:                    aconfig.Datadog.GetBool("runtime_security_config.self_test.enabled"),
		EnableRemoteConfig:                 aconfig.Datadog.GetBool("runtime_security_config.enable_remote_configuration"),
		EnableRuntimeCompiledConstants:     aconfig.Datadog.GetBool("runtime_security_config.enable_runtime_compiled_constants"),
		RuntimeCompiledConstantsIsSet:      aconfig.Datadog.IsSet("runtime_security_config.enable_runtime_compiled_constants"),
		ActivityDumpEnabled:                aconfig.Datadog.GetBool("runtime_security_config.activity_dump_manager.enabled"),
		ActivityDumpCleanupPeriod:          time.Duration(aconfig.Datadog.GetInt("runtime_security_config.activity_dump_manager.cleanup_period")) * time.Second,
		RuntimeMonitor:                     aconfig.Datadog.GetBool("runtime_security_config.runtime_monitor.enabled"),
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

	if !c.Config.EnableRuntimeCompiler {
		c.EnableRuntimeCompiledConstants = false
	}

	serviceName := utils.GetTagValue("service", aconfig.GetConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}

	setEnv()
	return c, nil
}
