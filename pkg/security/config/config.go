// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// Minimum value for runtime_security_config.activity_dump.max_dump_size
	ADMinMaxDumSize = 100
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// RuntimeSecurityConfig holds the configuration for the runtime security agent
type RuntimeSecurityConfig struct {
	// RuntimeEnabled defines if the runtime security module should be enabled
	RuntimeEnabled bool
	// PoliciesDir defines the folder in which the policy files are located
	PoliciesDir string
	// WatchPoliciesDir activate policy dir inotify
	WatchPoliciesDir bool
	// PolicyMonitorEnabled enable policy monitoring
	PolicyMonitorEnabled bool
	// SocketPath is the path to the socket that is used to communicate with the security agent
	SocketPath string
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
	// RemoteConfigurationEnabled defines whether to use remote monitoring
	RemoteConfigurationEnabled bool
	// LogPatterns pattern to be used by the logger for trace level
	LogPatterns []string
	// LogTags tags to be used by the logger for trace level
	LogTags []string
	// HostServiceName string
	HostServiceName string

	// AgentMonitoringEvents determines if the monitoring events of the agent should be sent to Datadog
	CustomEventEnabled bool

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

	// SecurityProfileEnabled defines if the Security Profile manager should be enabled
	SecurityProfileEnabled bool
	// SecurityProfileDir defines the directory in which Security Profiles are stored
	SecurityProfileDir string
	// SecurityProfileWatchDir defines if the Security Profiles directory should be monitored
	SecurityProfileWatchDir bool
	// SecurityProfileCacheSize defines the count of Security Profiles held in cache
	SecurityProfileCacheSize int

	// SBOMResolverEnabled defines if the SBOM resolver should be enabled
	SBOMResolverEnabled bool
	// SBOMResolverWorkloadsCacheSize defines the count of SBOMs to keep in memory in order to prevent re-computing
	// the SBOMs of short-lived and periodical workloads
	SBOMResolverWorkloadsCacheSize int
}

// Config defines a security config
type Config struct {
	// Probe Config
	Probe *pconfig.Config

	// CWS specific parameters
	RuntimeSecurity *RuntimeSecurityConfig
}

// NewConfig returns a new Config object
func NewConfig() (*Config, error) {
	probeConfig, err := pconfig.NewConfig()
	if err != nil {
		return nil, err
	}

	rsConfig, err := NewRuntimeSecurityConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		Probe:           probeConfig,
		RuntimeSecurity: rsConfig,
	}, nil
}

func NewRuntimeSecurityConfig() (*RuntimeSecurityConfig, error) {
	rsConfig := &RuntimeSecurityConfig{
		RuntimeEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.enabled"),
		FIMEnabled:     coreconfig.SystemProbe.GetBool("runtime_security_config.fim_enabled"),

		SocketPath:           coreconfig.SystemProbe.GetString("runtime_security_config.socket"),
		EventServerBurst:     coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:      coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention: coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.retention"),

		SelfTestEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.enabled"),
		SelfTestSendReport:         coreconfig.SystemProbe.GetBool("runtime_security_config.self_test.send_report"),
		RemoteConfigurationEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.remote_configuration.enabled"),

		// policy & ruleset
		PoliciesDir:          coreconfig.SystemProbe.GetString("runtime_security_config.policies.dir"),
		WatchPoliciesDir:     coreconfig.SystemProbe.GetBool("runtime_security_config.policies.watch_dir"),
		PolicyMonitorEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.policies.monitor.enabled"),

		LogPatterns: coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_patterns"),
		LogTags:     coreconfig.SystemProbe.GetStringSlice("runtime_security_config.log_tags"),

		// custom events
		CustomEventEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.agent_monitoring_events"),

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
		ActivityDumpTagRulesEnabled:           coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.tag_rules.enabled"),
		// activity dump dynamic fields
		ActivityDumpMaxDumpSize: func() int {
			mds := coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.max_dump_size")
			if mds < ADMinMaxDumSize {
				mds = ADMinMaxDumSize
			}
			return mds * (1 << 10)
		},

		// SBOM resolver
		SBOMResolverEnabled:            coreconfig.SystemProbe.GetBool("runtime_security_config.sbom.enabled"),
		SBOMResolverWorkloadsCacheSize: coreconfig.SystemProbe.GetInt("runtime_security_config.sbom.workloads_cache_size"),

		// security profiles
		SecurityProfileEnabled:   coreconfig.SystemProbe.GetBool("runtime_security_config.security_profile.enabled"),
		SecurityProfileDir:       coreconfig.SystemProbe.GetString("runtime_security_config.security_profile.dir"),
		SecurityProfileWatchDir:  coreconfig.SystemProbe.GetBool("runtime_security_config.security_profile.watch_dir"),
		SecurityProfileCacheSize: coreconfig.SystemProbe.GetInt("runtime_security_config.security_profile.cache_size"),
	}

	if err := rsConfig.sanitize(); err != nil {
		return nil, err
	}

	return rsConfig, nil
}

// IsRuntimeEnabled returns true if any feature is enabled. Has to be applied in config package too
func (c *RuntimeSecurityConfig) IsRuntimeEnabled() bool {
	return c.RuntimeEnabled || c.FIMEnabled
}

// sanitize ensures that the configuration is properly setup
func (c *RuntimeSecurityConfig) sanitize() error {
	// if runtime is enabled then we force fim
	if c.RuntimeEnabled {
		c.FIMEnabled = true
	}

	// if runtime is disabled then we force disable activity dumps
	if !c.RuntimeEnabled {
		c.ActivityDumpEnabled = false
	}

	serviceName := utils.GetTagValue("service", coreconfig.GetGlobalConfiguredTags(true))
	if len(serviceName) > 0 {
		c.HostServiceName = fmt.Sprintf("service:%s", serviceName)
	}

	return c.sanitizeRuntimeSecurityConfigActivityDump()
}

// sanitizeNetworkConfiguration ensures that runtime_security_config.activity_dump is properly configured
func (c *RuntimeSecurityConfig) sanitizeRuntimeSecurityConfigActivityDump() error {
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
