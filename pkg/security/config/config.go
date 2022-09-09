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
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
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
	// WatchPoliciesDir activate policy dir inotify
	WatchPoliciesDir bool
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
	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int
	// EventServerRate defines the grpc server rate at which events can be sent
	EventServerRate int
	// EventServerRetention defines an event retention period so that some fields can be resolved
	EventServerRetention int
	// PIDCacheSize is the size of the user space PID caches
	PIDCacheSize int
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
	// SelfTestEnabled defines if the self tests should be executed at startup or not
	SelfTestEnabled bool
	// SelfTestSendReport defines if a self test event will be emitted
	SelfTestSendReport bool
	// EnvsWithValue lists environnement variables that will be fully exported
	EnvsWithValue []string

	// ActivityDumpEnabled defines if the activity dump manager should be enabled
	ActivityDumpEnabled bool
	// ActivityDumpCleanupPeriod defines the period at which the activity dump manager should perform its cleanup
	// operation.
	ActivityDumpCleanupPeriod time.Duration
	// ActivityDumpTagsResolutionPeriod defines the period at which the activity dump manager should try to resolve
	// missing container tags.
	ActivityDumpTagsResolutionPeriod time.Duration
	// ActivityDumpLoadControlPeriod defines the period at which the activity dump manager should trigger the load controller
	ActivityDumpLoadControlPeriod time.Duration
	// ActivityDumpMaxDumpSize defines the maximum size of a dump
	ActivityDumpMaxDumpSize int
	// ActivityDumpPathMergeEnabled defines if path merge should be enabled
	ActivityDumpPathMergeEnabled bool
	// ActivityDumpTracedCgroupsCount defines the maximum count of cgroups that should be monitored concurrently. Leave this parameter to 0 to prevent the generation
	// of activity dumps based on cgroups.
	ActivityDumpTracedCgroupsCount int
	// ActivityDumpTracedEventTypes defines the list of events that should be captured in an activity dump. Leave this
	// parameter empty to monitor all event types. If not already present, the `exec` event will automatically be added
	// to this list.
	ActivityDumpTracedEventTypes []model.EventType
	// ActivityDumpCgroupDumpTimeout defines the cgroup activity dumps timeout.
	ActivityDumpCgroupDumpTimeout time.Duration
	// ActivityDumpRateLimiter defines the kernel rate of max events per sec for activity dumps.
	ActivityDumpRateLimiter int
	// ActivityDumpCgroupWaitListSize defines the size of the cgroup wait list. The wait list is used to introduce a
	// delay between 2 activity dumps of the same cgroup.
	ActivityDumpCgroupWaitListSize int
	// ActivityDumpCgroupDifferentiateArgs defines if system-probe should differentiate process nodes using process
	// arguments for dumps.
	ActivityDumpCgroupDifferentiateArgs bool
	// ActivityDumpLocalStorageDirectory defines the output directory for the activity dumps and graphs. Leave
	// this field empty to prevent writing any output to disk.
	ActivityDumpLocalStorageDirectory string
	// ActivityDumpLocalStorageFormats defines the formats that should be used to persist the activity dumps locally.
	ActivityDumpLocalStorageFormats []dump.StorageFormat
	// ActivityDumpLocalStorageCompression defines if the local storage should compress the persisted data.
	ActivityDumpLocalStorageCompression bool
	// ActivityDumpLocalStorageMaxDumpsCount defines the maximum count of activity dumps that should be kept locally.
	// When the limit is reached, the oldest dumps will be deleted first.
	ActivityDumpLocalStorageMaxDumpsCount int
	// ActivityDumpRemoteStorageFormats defines the formats that should be used to persist the activity dumps remotely.
	ActivityDumpRemoteStorageFormats []dump.StorageFormat
	// ActivityDumpRemoteStorageCompression defines if the remote storage should compress the persisted data.
	ActivityDumpRemoteStorageCompression bool
	// ActivityDumpSyscallMonitor defines if activity dumps should collect syscalls or not
	ActivityDumpSyscallMonitor bool
	// ActivityDumpSyscallMonitorPeriod defines the minimum amount of time to wait between 2 syscalls event for the same
	// process.
	ActivityDumpSyscallMonitorPeriod time.Duration

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
	// EventMonitoring enables event monitoring
	EventMonitoring bool
	// RemoteConfigurationEnabled defines whether to use remote monitoring
	RemoteConfigurationEnabled bool
	// EventStreamUseRingBuffer specifies whether to use eBPF ring buffers when available
	EventStreamUseRingBuffer bool
	// EventStreamBufferSize specifies the buffer size of the eBPF map used for events
	EventStreamBufferSize int
}

// IsEnabled returns true if any feature is enabled. Has to be applied in config package too
func (c *Config) IsEnabled() bool {
	return c.RuntimeEnabled || c.FIMEnabled
}

func setEnv() {
	if coreconfig.IsContainerized() && util.PathExists("/host") {
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
		RuntimeEnabled:                     coreconfig.Datadog.GetBool("runtime_security_config.enabled"),
		FIMEnabled:                         coreconfig.Datadog.GetBool("runtime_security_config.fim_enabled"),
		EventMonitoring:                    coreconfig.Datadog.GetBool("runtime_security_config.event_monitoring.enabled"),
		EnableKernelFilters:                coreconfig.Datadog.GetBool("runtime_security_config.enable_kernel_filters"),
		EnableApprovers:                    coreconfig.Datadog.GetBool("runtime_security_config.enable_approvers"),
		EnableDiscarders:                   coreconfig.Datadog.GetBool("runtime_security_config.enable_discarders"),
		FlushDiscarderWindow:               coreconfig.Datadog.GetInt("runtime_security_config.flush_discarder_window"),
		SocketPath:                         coreconfig.Datadog.GetString("runtime_security_config.socket"),
		PoliciesDir:                        coreconfig.Datadog.GetString("runtime_security_config.policies.dir"),
		WatchPoliciesDir:                   coreconfig.Datadog.GetBool("runtime_security_config.policies.watch_dir"),
		EventServerBurst:                   coreconfig.Datadog.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:                    coreconfig.Datadog.GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention:               coreconfig.Datadog.GetInt("runtime_security_config.event_server.retention"),
		PIDCacheSize:                       coreconfig.Datadog.GetInt("runtime_security_config.pid_cache_size"),
		LoadControllerEventsCountThreshold: int64(coreconfig.Datadog.GetInt("runtime_security_config.load_controller.events_count_threshold")),
		LoadControllerDiscarderTimeout:     time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.load_controller.discarder_timeout")) * time.Second,
		LoadControllerControlPeriod:        time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.load_controller.control_period")) * time.Second,
		StatsPollingInterval:               time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.events_stats.polling_interval")) * time.Second,
		StatsTagsCardinality:               coreconfig.Datadog.GetString("runtime_security_config.events_stats.tags_cardinality"),
		StatsdAddr:                         fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort),
		AgentMonitoringEvents:              coreconfig.Datadog.GetBool("runtime_security_config.agent_monitoring_events"),
		CustomSensitiveWords:               coreconfig.Datadog.GetStringSlice("runtime_security_config.custom_sensitive_words"),
		ERPCDentryResolutionEnabled:        coreconfig.Datadog.GetBool("runtime_security_config.erpc_dentry_resolution_enabled"),
		MapDentryResolutionEnabled:         coreconfig.Datadog.GetBool("runtime_security_config.map_dentry_resolution_enabled"),
		DentryCacheSize:                    coreconfig.Datadog.GetInt("runtime_security_config.dentry_cache_size"),
		RemoteTaggerEnabled:                coreconfig.Datadog.GetBool("runtime_security_config.remote_tagger"),
		LogPatterns:                        coreconfig.Datadog.GetStringSlice("runtime_security_config.log_patterns"),
		LogTags:                            coreconfig.Datadog.GetStringSlice("runtime_security_config.log_tags"),
		SelfTestEnabled:                    coreconfig.Datadog.GetBool("runtime_security_config.self_test.enabled"),
		SelfTestSendReport:                 coreconfig.Datadog.GetBool("runtime_security_config.self_test.send_report"),
		RuntimeMonitor:                     coreconfig.Datadog.GetBool("runtime_security_config.runtime_monitor.enabled"),
		NetworkEnabled:                     coreconfig.Datadog.GetBool("runtime_security_config.network.enabled"),
		NetworkLazyInterfacePrefixes:       coreconfig.Datadog.GetStringSlice("runtime_security_config.network.lazy_interface_prefixes"),
		RemoteConfigurationEnabled:         coreconfig.Datadog.GetBool("runtime_security_config.remote_configuration.enabled"),
		EventStreamUseRingBuffer:           coreconfig.Datadog.GetBool("runtime_security_config.event_stream.use_ring_buffer"),
		EventStreamBufferSize:              coreconfig.Datadog.GetInt("runtime_security_config.event_stream.buffer_size"),
		EnvsWithValue:                      coreconfig.Datadog.GetStringSlice("runtime_security_config.envs_with_value"),

		// runtime compilation
		RuntimeCompilationEnabled:       coreconfig.Datadog.GetBool("runtime_security_config.runtime_compilation.enabled"),
		RuntimeCompiledConstantsEnabled: coreconfig.Datadog.GetBool("runtime_security_config.runtime_compilation.compiled_constants_enabled"),
		RuntimeCompiledConstantsIsSet:   coreconfig.Datadog.IsSet("runtime_security_config.runtime_compilation.compiled_constants_enabled"),

		// activity dump
		ActivityDumpEnabled:                   coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.enabled"),
		ActivityDumpCleanupPeriod:             time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.cleanup_period")) * time.Second,
		ActivityDumpTagsResolutionPeriod:      time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.tags_resolution_period")) * time.Second,
		ActivityDumpLoadControlPeriod:         time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.load_controller_period")) * time.Minute,
		ActivityDumpMaxDumpSize:               coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.max_dump_size") * (1 << 10),
		ActivityDumpPathMergeEnabled:          coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.path_merge.enabled"),
		ActivityDumpTracedCgroupsCount:        coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.traced_cgroups_count"),
		ActivityDumpTracedEventTypes:          model.ParseEventTypeStringSlice(coreconfig.Datadog.GetStringSlice("runtime_security_config.activity_dump.traced_event_types")),
		ActivityDumpCgroupDumpTimeout:         time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.cgroup_dump_timeout")) * time.Minute,
		ActivityDumpRateLimiter:               coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.rate_limiter"),
		ActivityDumpCgroupWaitListSize:        coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.cgroup_wait_list_size"),
		ActivityDumpCgroupDifferentiateArgs:   coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.cgroup_differentiate_args"),
		ActivityDumpLocalStorageDirectory:     coreconfig.Datadog.GetString("runtime_security_config.activity_dump.local_storage.output_directory"),
		ActivityDumpLocalStorageMaxDumpsCount: coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.local_storage.max_dumps_count"),
		ActivityDumpLocalStorageCompression:   coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.local_storage.compression"),
		ActivityDumpRemoteStorageCompression:  coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.remote_storage.compression"),
		ActivityDumpSyscallMonitor:            coreconfig.Datadog.GetBool("runtime_security_config.activity_dump.syscall_monitor.enabled"),
		ActivityDumpSyscallMonitorPeriod:      time.Duration(coreconfig.Datadog.GetInt("runtime_security_config.activity_dump.syscall_monitor.period")) * time.Second,
	}

	if err := c.sanitize(); err != nil {
		return nil, fmt.Errorf("invalid CWS configuration: %w", err)
	}

	setEnv()
	return c, nil
}

// sanitize ensures that the configuration is properly setup
func (c *Config) sanitize() error {
	// if runtime is enabled then we force fim
	if c.RuntimeEnabled {
		c.FIMEnabled = true
	}

	if !c.IsEnabled() {
		return nil
	}

	if !coreconfig.Datadog.IsSet("runtime_security_config.enable_approvers") && c.EnableKernelFilters {
		c.EnableApprovers = true
	}

	if !coreconfig.Datadog.IsSet("runtime_security_config.enable_discarders") && c.EnableKernelFilters {
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

	serviceName := utils.GetTagValue("service", coreconfig.GetConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}

	if c.EventStreamBufferSize%os.Getpagesize() != 0 || c.EventStreamBufferSize&(c.EventStreamBufferSize-1) != 0 {
		return fmt.Errorf("runtime_security_config.event_stream.buffer_size must be a power of 2 and a multiple of %d", os.Getpagesize())
	}

	c.sanitizeRuntimeSecurityConfigNetwork()
	return c.sanitizeRuntimeSecurityConfigActivityDump()
}

// sanitizeNetworkConfiguration ensures that runtime_security_config.network is properly configured
func (c *Config) sanitizeRuntimeSecurityConfigNetwork() {
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
}

// sanitizeNetworkConfiguration ensures that runtime_security_config.activity_dump is properly configured
func (c *Config) sanitizeRuntimeSecurityConfigActivityDump() error {
	var execFound bool
	for _, evtType := range c.ActivityDumpTracedEventTypes {
		switch evtType {
		case model.ExecEventType:
			execFound = true
		case model.SyscallsEventType:
			// enable the syscall monitor
			c.ActivityDumpSyscallMonitor = true
		}
	}
	if !execFound {
		c.ActivityDumpTracedEventTypes = append(c.ActivityDumpTracedEventTypes, model.ExecEventType)
	}

	if formats := coreconfig.Datadog.GetStringSlice("runtime_security_config.activity_dump.local_storage.formats"); len(formats) > 0 {
		var err error
		c.ActivityDumpLocalStorageFormats, err = dump.ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.local_storage.formats: %w", err)
		}
	}
	if formats := coreconfig.Datadog.GetStringSlice("runtime_security_config.activity_dump.remote_storage.formats"); len(formats) > 0 {
		var err error
		c.ActivityDumpRemoteStorageFormats, err = dump.ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.remote_storage.formats: %w", err)
		}
	}

	if c.ActivityDumpTracedCgroupsCount > probes.MaxTracedCgroupsCount {
		c.ActivityDumpTracedCgroupsCount = probes.MaxTracedCgroupsCount
	}

	if c.ActivityDumpCgroupWaitListSize <= 0 {
		c.ActivityDumpCgroupWaitListSize = c.ActivityDumpTracedCgroupsCount
	}

	if c.ActivityDumpCgroupWaitListSize > probes.MaxTracedCgroupsCount {
		c.ActivityDumpCgroupWaitListSize = probes.MaxTracedCgroupsCount
	}
	return nil
}

// ActivityDumpRemoteStorageEndpoints returns the list of activity dump remote storage endpoints parsed from the agent config
func ActivityDumpRemoteStorageEndpoints(endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	logsConfig := logsconfig.NewLogsConfigKeys("runtime_security_config.activity_dump.remote_storage.endpoints.", coreconfig.Datadog)
	endpoints, err := logsconfig.BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		endpoints, err = logsconfig.BuildHTTPEndpoints(intakeTrackType, intakeProtocol, intakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = logsconfig.BuildEndpoints(httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("invalid endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		seclog.Infof("activity dump remote storage endpoint: %v\n", status)
	}
	return endpoints, nil
}
