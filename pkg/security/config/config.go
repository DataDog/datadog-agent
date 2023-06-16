// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
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
	EventServerRetention time.Duration
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
	// ActivityDumpLoadControlMinDumpTimeout defines minimal duration of a activity dump recording
	ActivityDumpLoadControlMinDumpTimeout time.Duration

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
	// ActivityDumpSyscallMonitorPeriod defines the minimum amount of time to wait between 2 syscalls event for the same
	// process.
	ActivityDumpSyscallMonitorPeriod time.Duration
	// ActivityDumpMaxDumpCountPerWorkload defines the maximum amount of dumps that the agent should send for a workload
	ActivityDumpMaxDumpCountPerWorkload int
	// ActivityDumpTagRulesEnabled enable the tagging of nodes with matched rules (only for rules having the tag ruleset:threat_score)
	ActivityDumpTagRulesEnabled bool
	// ActivityDumpSilentWorkloadsDelay defines the minimum amount of time to wait before the activity dump manager will start tracing silent workloads
	ActivityDumpSilentWorkloadsDelay time.Duration
	// ActivityDumpSilentWorkloadsTicker configures ticker that will check if a workload is silent and should be traced
	ActivityDumpSilentWorkloadsTicker time.Duration

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
	// SecurityProfileMaxCount defines the maximum number of Security Profiles that may be evaluated concurrently
	SecurityProfileMaxCount int
	// SecurityProfileRCEnabled defines if remote-configuration is enabled
	SecurityProfileRCEnabled bool
	// SecurityProfileDNSMatchMaxDepth defines the max depth of subdomain to be matched for DNS anomaly detection (0 to match everything)
	SecurityProfileDNSMatchMaxDepth int

	// AnomalyDetectionEventTypes defines the list of events that should be allowed to generate anomaly detections
	AnomalyDetectionEventTypes []model.EventType
	// AnomalyDetectionMinimumStablePeriod defines the minimum amount of time during which the events
	// that diverge from their profiles are automatically added in their profiles without triggering an anomaly detection
	// event.
	AnomalyDetectionMinimumStablePeriod time.Duration
	// AnomalyDetectionUnstableProfileTimeThreshold defines the maximum amount of time to wait until a profile that
	// hasn't reached a stable state is considered as unstable.
	AnomalyDetectionUnstableProfileTimeThreshold time.Duration
	// AnomalyDetectionUnstableProfileSizeThreshold defines the maximum size a profile can reach past which it is
	// considered unstable
	AnomalyDetectionUnstableProfileSizeThreshold int64
	// AnomalyDetectionWorkloadWarmupPeriod defines the duration we ignore the anomaly detections for
	// because of workload warm up
	AnomalyDetectionWorkloadWarmupPeriod time.Duration
	// AnomalyDetectionRateLimiter limit number of anomaly event, one every N second
	AnomalyDetectionRateLimiter time.Duration

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
	sysconfig.Adjust(coreconfig.SystemProbe)

	rsConfig := &RuntimeSecurityConfig{
		RuntimeEnabled: coreconfig.SystemProbe.GetBool("runtime_security_config.enabled"),
		FIMEnabled:     coreconfig.SystemProbe.GetBool("runtime_security_config.fim_enabled"),

		SocketPath:           coreconfig.SystemProbe.GetString("runtime_security_config.socket"),
		EventServerBurst:     coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.burst"),
		EventServerRate:      coreconfig.SystemProbe.GetInt("runtime_security_config.event_server.rate"),
		EventServerRetention: coreconfig.SystemProbe.GetDuration("runtime_security_config.event_server.retention"),

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
		ActivityDumpCleanupPeriod:             coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.cleanup_period"),
		ActivityDumpTagsResolutionPeriod:      coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.tags_resolution_period"),
		ActivityDumpLoadControlPeriod:         coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.load_controller_period"),
		ActivityDumpLoadControlMinDumpTimeout: coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.min_timeout"),
		ActivityDumpTracedCgroupsCount:        coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.traced_cgroups_count"),
		ActivityDumpTracedEventTypes:          model.ParseEventTypeStringSlice(coreconfig.SystemProbe.GetStringSlice("runtime_security_config.activity_dump.traced_event_types")),
		ActivityDumpCgroupDumpTimeout:         coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.dump_duration"),
		ActivityDumpRateLimiter:               coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.rate_limiter"),
		ActivityDumpCgroupWaitListTimeout:     coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.cgroup_wait_list_timeout"),
		ActivityDumpCgroupDifferentiateArgs:   coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.cgroup_differentiate_args"),
		ActivityDumpLocalStorageDirectory:     coreconfig.SystemProbe.GetString("runtime_security_config.activity_dump.local_storage.output_directory"),
		ActivityDumpLocalStorageMaxDumpsCount: coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.local_storage.max_dumps_count"),
		ActivityDumpLocalStorageCompression:   coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.local_storage.compression"),
		ActivityDumpSyscallMonitorPeriod:      coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.syscall_monitor.period"),
		ActivityDumpMaxDumpCountPerWorkload:   coreconfig.SystemProbe.GetInt("runtime_security_config.activity_dump.max_dump_count_per_workload"),
		ActivityDumpTagRulesEnabled:           coreconfig.SystemProbe.GetBool("runtime_security_config.activity_dump.tag_rules.enabled"),
		ActivityDumpSilentWorkloadsDelay:      coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.silent_workloads.delay"),
		ActivityDumpSilentWorkloadsTicker:     coreconfig.SystemProbe.GetDuration("runtime_security_config.activity_dump.silent_workloads.ticker"),
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
		SecurityProfileEnabled:          coreconfig.SystemProbe.GetBool("runtime_security_config.security_profile.enabled"),
		SecurityProfileDir:              coreconfig.SystemProbe.GetString("runtime_security_config.security_profile.dir"),
		SecurityProfileWatchDir:         coreconfig.SystemProbe.GetBool("runtime_security_config.security_profile.watch_dir"),
		SecurityProfileCacheSize:        coreconfig.SystemProbe.GetInt("runtime_security_config.security_profile.cache_size"),
		SecurityProfileMaxCount:         coreconfig.SystemProbe.GetInt("runtime_security_config.security_profile.max_count"),
		SecurityProfileRCEnabled:        coreconfig.SystemProbe.GetBool("runtime_security_config.security_profile.remote_configuration.enabled"),
		SecurityProfileDNSMatchMaxDepth: coreconfig.SystemProbe.GetInt("runtime_security_config.security_profile.dns_match_max_depth"),

		// anomaly detection
		AnomalyDetectionEventTypes:                   model.ParseEventTypeStringSlice(coreconfig.SystemProbe.GetStringSlice("runtime_security_config.security_profile.anomaly_detection.event_types")),
		AnomalyDetectionMinimumStablePeriod:          coreconfig.SystemProbe.GetDuration("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period"),
		AnomalyDetectionWorkloadWarmupPeriod:         coreconfig.SystemProbe.GetDuration("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period"),
		AnomalyDetectionUnstableProfileTimeThreshold: coreconfig.SystemProbe.GetDuration("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold"),
		AnomalyDetectionUnstableProfileSizeThreshold: coreconfig.SystemProbe.GetInt64("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold"),
		AnomalyDetectionRateLimiter:                  coreconfig.SystemProbe.GetDuration("runtime_security_config.security_profile.anomaly_detection.rate_limiter"),
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
