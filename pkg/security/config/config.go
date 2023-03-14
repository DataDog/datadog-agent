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
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// Minimum value for runtime_security_config.activity_dump.max_dump_size
	MinMaxDumSize = 100
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

type EventMonitorConfig struct {
	ebpf.Config

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
	// NOTE(safchain) need to revisit this one as it can impact multiple event consumers
	// EnvsWithValue lists environnement variables that will be fully exported
	EnvsWithValue []string
	// RuntimeMonitor defines if the Go runtime and system monitor should be enabled
	RuntimeMonitor bool
	// EventStreamUseRingBuffer specifies whether to use eBPF ring buffers when available
	EventStreamUseRingBuffer bool
	// EventStreamBufferSize specifies the buffer size of the eBPF map used for events
	EventStreamBufferSize int
	// RuntimeCompilationEnabled defines if the runtime-compilation is enabled
	RuntimeCompilationEnabled bool
	// EnableRuntimeCompiledConstants defines if the runtime compilation based constant fetcher is enabled
	RuntimeCompiledConstantsEnabled bool
	// RuntimeCompiledConstantsIsSet is set if the runtime compiled constants option is user-set
	RuntimeCompiledConstantsIsSet bool
	// EventMonitoring enables event monitoring. Send events to external consumer.
	EventMonitoring bool
	// LogPatterns pattern to be used by the logger for trace level
	LogPatterns []string
	// LogTags tags to be used by the logger for trace level
	LogTags []string
	// NetworkEnabled defines if the network probes should be activated
	NetworkEnabled bool
	// NetworkLazyInterfacePrefixes is the list of interfaces prefix that aren't explicitly deleted by the container
	// runtime, and that are lazily deleted by the kernel when a network namespace is cleaned up. This list helps the
	// agent detect when a network namespace should be purged from all caches.
	NetworkLazyInterfacePrefixes []string
	// NetworkClassifierPriority defines the priority at which CWS should insert its TC classifiers.
	NetworkClassifierPriority uint16
	// NetworkClassifierHandle defines the handle at which CWS should insert its TC classifiers.
	NetworkClassifierHandle uint16
}

type ProcessEventMonitoringConfig struct {
	Enabled bool
}

type NetworkProcessEventMonitoringConfig struct {
	Enabled bool
}

type CWSConfig struct {
	// RuntimeEnabled defines if the runtime security module should be enabled
	RuntimeEnabled bool
	// PoliciesDir defines the folder in which the policy files are located
	PoliciesDir string
	// WatchPoliciesDir activate policy dir inotify
	WatchPoliciesDir bool
	// PolicyMonitorEnabled enable policy monitoring
	PolicyMonitorEnabled bool
	// EventServerBurst defines the maximum burst of events that can be sent over the grpc server
	EventServerBurst int
	// EventServerRate defines the grpc server rate at which events can be sent
	EventServerRate int
	// EventServerRetention defines an event retention period so that some fields can be resolved
	EventServerRetention int
	// FIMEnabled determines whether fim rules will be loaded
	FIMEnabled bool
	// SelfTestEnabled defines if the self tests should be executed at startup or not
	SelfTestEnabled bool
	// SelfTestSendReport defines if a self test event will be emitted
	SelfTestSendReport bool
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
	// ActivityDumpCgroupWaitListTimeout defines the time to wait before a cgroup can be dumped again.
	ActivityDumpCgroupWaitListTimeout time.Duration
	// ActivityDumpCgroupDifferentiateArgs defines if system-probe should differentiate process nodes using process
	// arguments for dumps.
	ActivityDumpCgroupDifferentiateArgs bool
	// ActivityDumpLocalStorageDirectory defines the output directory for the activity dumps and graphs. Leave
	// this field empty to prevent writing any output to disk.
	ActivityDumpLocalStorageDirectory string
	// ActivityDumpLocalStorageFormats defines the formats that should be used to persist the activity dumps locally.
	ActivityDumpLocalStorageFormats []StorageFormat
	// ActivityDumpLocalStorageCompression defines if the local storage should compress the persisted data.
	ActivityDumpLocalStorageCompression bool
	// ActivityDumpLocalStorageMaxDumpsCount defines the maximum count of activity dumps that should be kept locally.
	// When the limit is reached, the oldest dumps will be deleted first.
	ActivityDumpLocalStorageMaxDumpsCount int
	// ActivityDumpRemoteStorageFormats defines the formats that should be used to persist the activity dumps remotely.
	ActivityDumpRemoteStorageFormats []StorageFormat
	// ActivityDumpSyscallMonitorPeriod defines the minimum amount of time to wait between 2 syscalls event for the same
	// process.
	ActivityDumpSyscallMonitorPeriod time.Duration
	// ActivityDumpMaxDumpCountPerWorkload defines the maximum amount of dumps that the agent should send for a workload
	ActivityDumpMaxDumpCountPerWorkload int
	// ActivityDumpTagRulesEnabled enable the tagging of nodes with matched rules (only for rules having the tag ruleset:thread_score)
	ActivityDumpTagRulesEnabled bool

	// # Dynamic configuration fields:
	// ActivityDumpMaxDumpSize defines the maximum size of a dump
	ActivityDumpMaxDumpSize func() int
	// RemoteConfigurationEnabled defines whether to use remote monitoring
	RemoteConfigurationEnabled bool

	// SBOMResolverEnabled defines if the SBOM resolver should be enabled
	SBOMResolverEnabled bool
	// SBOMResolverWorkloadsCacheSize defines the count of SBOMs to keep in memory in order to prevent re-computing
	// the SBOMs of short-lived and periodical workloads
	SBOMResolverWorkloadsCacheSize int
}

// Config holds the configuration for the runtime security agent
type Config struct {
	EventMonitorConfig
	CWSConfig
	ProcessEventMonitoringConfig
	NetworkProcessEventMonitoringConfig
}

// IsRuntimeEnabled returns true if any feature is enabled. Has to be applied in config package too
func (c *Config) IsRuntimeEnabled() bool {
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
		EventMonitorConfig: EventMonitorConfig{
			Config:                             *ebpf.NewConfig(),
			EnableKernelFilters:                coreconfig.SystemProbe.GetBool("runtime_security_config.enable_kernel_filters"),
			EnableApprovers:                    coreconfig.SystemProbe.GetBool("runtime_security_config.enable_approvers"),
			EnableDiscarders:                   coreconfig.SystemProbe.GetBool("runtime_security_config.enable_discarders"),
			FlushDiscarderWindow:               coreconfig.SystemProbe.GetInt("runtime_security_config.flush_discarder_window"),
			SocketPath:                         coreconfig.SystemProbe.GetString("runtime_security_config.socket"),
			PIDCacheSize:                       coreconfig.SystemProbe.GetInt("runtime_security_config.pid_cache_size"),
			LoadControllerEventsCountThreshold: int64(coreconfig.SystemProbe.GetInt("runtime_security_config.load_controller.events_count_threshold")),
			LoadControllerDiscarderTimeout:     time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.load_controller.discarder_timeout")) * time.Second,
			LoadControllerControlPeriod:        time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.load_controller.control_period")) * time.Second,
			StatsPollingInterval:               time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.events_stats.polling_interval")) * time.Second,
			StatsTagsCardinality:               coreconfig.SystemProbe.GetString("runtime_security_config.events_stats.tags_cardinality"),
			StatsdAddr:                         fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort),
			AgentMonitoringEvents:              coreconfig.SystemProbe.GetBool("runtime_security_config.agent_monitoring_events"),
			CustomSensitiveWords:               coreconfig.SystemProbe.GetStringSlice("runtime_security_config.custom_sensitive_words"),
			ERPCDentryResolutionEnabled:        coreconfig.SystemProbe.GetBool("runtime_security_config.erpc_dentry_resolution_enabled"),
			MapDentryResolutionEnabled:         coreconfig.SystemProbe.GetBool("runtime_security_config.map_dentry_resolution_enabled"),
			DentryCacheSize:                    coreconfig.SystemProbe.GetInt("runtime_security_config.dentry_cache_size"),
			RemoteTaggerEnabled:                coreconfig.SystemProbe.GetBool("runtime_security_config.remote_tagger"),
			LogPatterns:                        coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_patterns"),
			LogTags:                            coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_tags"),
			RuntimeMonitor:                     coreconfig.SystemProbe.GetBool("runtime_security_config.runtime_monitor.enabled"),
			NetworkEnabled:                     coreconfig.SystemProbe.GetBool("runtime_security_config.network.enabled"),
			NetworkLazyInterfacePrefixes:       coreconfig.SystemProbe.GetStringSlice("runtime_security_config.network.lazy_interface_prefixes"),
			NetworkClassifierPriority:          uint16(coreconfig.SystemProbe.GetInt("runtime_security_config.network.classifier_priority")),
			NetworkClassifierHandle:            uint16(coreconfig.SystemProbe.GetInt("runtime_security_config.network.classifier_handle")),
			EventStreamUseRingBuffer:           coreconfig.SystemProbe.GetBool("runtime_security_config.event_stream.use_ring_buffer"),
			EventStreamBufferSize:              coreconfig.SystemProbe.GetInt("runtime_security_config.event_stream.buffer_size"),
			EnvsWithValue:                      coreconfig.SystemProbe.GetStringSlice("runtime_security_config.envs_with_value"),
			// runtime compilation
			RuntimeCompilationEnabled:       coreconfig.SystemProbe.GetBool("runtime_security_config.runtime_compilation.enabled"),
			RuntimeCompiledConstantsEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.runtime_compilation.compiled_constants_enabled"),
			RuntimeCompiledConstantsIsSet:   coreconfig.SystemProbe.IsSet("runtime_security_config.runtime_compilation.compiled_constants_enabled"),
		},
		CWSConfig: CWSConfig{
			RuntimeEnabled:             coreconfig.SystemProbe.GetBool("runtime_security_config.enabled"),
			FIMEnabled:                 coreconfig.SystemProbe.GetBool("runtime_security_config.fim_enabled"),
			SelfTestEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.enabled"),
			SelfTestSendReport:         coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.send_report"),
			RemoteConfigurationEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.remote_configuration.enabled"),

			EventServerBurst:     coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.burst"),
			EventServerRate:      coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.rate"),
			EventServerRetention: coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.retention"),

			// policy & ruleset
			PoliciesDir:          coreconfig.SystemProbe.GetString("runtime_security_config.policies.dir"),
			WatchPoliciesDir:     coreconfig.SystemProbe.GetBool("runtime_security_config.policies.watch_dir"),
			PolicyMonitorEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.policies.monitor.enabled"),
			// activity dump
			ActivityDumpEnabled:                   coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.enabled"),
			ActivityDumpCleanupPeriod:             time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.cleanup_period")) * time.Second,
			ActivityDumpTagsResolutionPeriod:      time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.tags_resolution_period")) * time.Second,
			ActivityDumpLoadControlPeriod:         time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.load_controller_period")) * time.Minute,
			ActivityDumpPathMergeEnabled:          coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.path_merge.enabled"),
			ActivityDumpTracedCgroupsCount:        coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.traced_cgroups_count"),
			ActivityDumpTracedEventTypes:          model.ParseEventTypeStringSlice(coreconfig.SystemProbe.GetStringSlice("runtime_security_config.activity_dump.traced_event_types")),
			ActivityDumpCgroupDumpTimeout:         time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.cgroup_dump_timeout")) * time.Minute,
			ActivityDumpRateLimiter:               coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.rate_limiter"),
			ActivityDumpCgroupWaitListTimeout:     time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.cgroup_wait_list_timeout")) * time.Minute,
			ActivityDumpCgroupDifferentiateArgs:   coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.cgroup_differentiate_args"),
			ActivityDumpLocalStorageDirectory:     coreconfig.SystemProbe.GetString("runtime_security_config.activity_dump.local_storage.output_directory"),
			ActivityDumpLocalStorageMaxDumpsCount: coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.local_storage.max_dumps_count"),
			ActivityDumpLocalStorageCompression:   coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.local_storage.compression"),
			ActivityDumpSyscallMonitorPeriod:      time.Duration(coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.syscall_monitor.period")) * time.Second,
			ActivityDumpMaxDumpCountPerWorkload:   coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.max_dump_count_per_workload"),
			// activity dump dynamic fields
			ActivityDumpMaxDumpSize: func() int {
				mds := coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.max_dump_size")
				if mds < MinMaxDumSize {
					mds = MinMaxDumSize
				}
				return mds * (1 << 10)
			},
			// SBOM resolver
			SBOMResolverEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.sbom.enabled"),
			SBOMResolverWorkloadsCacheSize: coreconfig.SystemProbe.GetInt("runtime_security_config.sbom.workloads_cache_size"),
		},
		ProcessEventMonitoringConfig: ProcessEventMonitoringConfig{
			Enabled: coreconfig.SystemProbe.GetBool("event_monitoring_config.process.enabled"),
		},
	}

	if err := c.sanitize(); err != nil {
		return nil, fmt.Errorf("invalid CWS configuration: %w", err)
	}

	setEnv()
	return c, nil
}

// sanitize global config parameters, process monitoring + runtime security
func (c *Config) globalSanitize() error {
	// place here everything that could be necessary for process monitoring
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

	serviceName := utils.GetTagValue("service", coreconfig.GetGlobalConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}

	if c.EventStreamBufferSize%os.Getpagesize() != 0 || c.EventStreamBufferSize&(c.EventStreamBufferSize-1) != 0 {
		return fmt.Errorf("runtime_security_config.event_stream.buffer_size must be a power of 2 and a multiple of %d", os.Getpagesize())
	}

	return nil
}

// sanitize runtime specific config parameters
func (c *Config) sanitizeRuntime() error {
	// if runtime is enabled then we force fim
	if c.RuntimeEnabled {
		c.FIMEnabled = true
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

	c.sanitizeRuntimeSecurityConfigNetwork()
	return c.sanitizeRuntimeSecurityConfigActivityDump()
}

// disable all the runtime features
func (c *Config) disableRuntime() {
	c.ActivityDumpEnabled = false
}

// sanitize ensures that the configuration is properly setup
func (c *Config) sanitize() error {
	if err := c.globalSanitize(); err != nil {
		return err
	}

	// the following config params
	if !c.IsRuntimeEnabled() {
		c.disableRuntime()
		return nil
	}

	return c.sanitizeRuntime()
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
		if evtType == model.ExecEventType {
			execFound = true
			break
		}
	}
	if !execFound {
		c.ActivityDumpTracedEventTypes = append(c.ActivityDumpTracedEventTypes, model.ExecEventType)
	}

	if formats := coreconfig.SystemProbe.GetStringSlice("runtime_security_config.activity_dump.local_storage.formats"); len(formats) > 0 {
		var err error
		c.ActivityDumpLocalStorageFormats, err = ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.local_storage.formats: %w", err)
		}
	}
	if formats := coreconfig.Datadog.GetStringSlice("runtime_security_config.activity_dump.remote_storage.formats"); len(formats) > 0 {
		var err error
		c.ActivityDumpRemoteStorageFormats, err = ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.remote_storage.formats: %w", err)
		}
	}

	if c.ActivityDumpTracedCgroupsCount > model.MaxTracedCgroupsCount {
		c.ActivityDumpTracedCgroupsCount = model.MaxTracedCgroupsCount
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
