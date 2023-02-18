package config

import (
	"fmt"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// Minimum value for runtime_security_config.activity_dump.max_dump_size
	MinMaxDumSize = 100

	adNS = "runtime_security_config.activity_dump"
)

type Config struct {
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

	// # Dynamic configuration fields:
	// ActivityDumpMaxDumpSize defines the maximum size of a dump
	ActivityDumpMaxDumpSize func() int
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

func NewConfig() (*Config, error) {
	cfg := coreconfig.SystemProbe
	c := &Config{
		ActivityDumpCleanupPeriod:             time.Duration(cfg.GetInt(join(adNS, "cleanup_period"))) * time.Second,
		ActivityDumpTagsResolutionPeriod:      time.Duration(cfg.GetInt(join(adNS, "tags_resolution_period"))) * time.Second,
		ActivityDumpLoadControlPeriod:         time.Duration(cfg.GetInt(join(adNS, "load_controller_period"))) * time.Minute,
		ActivityDumpPathMergeEnabled:          cfg.GetBool(join(adNS, "path_merge", "enabled")),
		ActivityDumpTracedCgroupsCount:        cfg.GetInt(join(adNS, "traced_cgroups_count")),
		ActivityDumpTracedEventTypes:          model.ParseEventTypeStringSlice(cfg.GetStringSlice(join(adNS, "traced_event_types"))),
		ActivityDumpCgroupDumpTimeout:         time.Duration(cfg.GetInt(join(adNS, "cgroup_dump_timeout"))) * time.Minute,
		ActivityDumpRateLimiter:               cfg.GetInt(join(adNS, "rate_limiter")),
		ActivityDumpCgroupWaitListTimeout:     time.Duration(cfg.GetInt(join(adNS, "cgroup_wait_list_timeout"))) * time.Minute,
		ActivityDumpCgroupDifferentiateArgs:   cfg.GetBool(join(adNS, "cgroup_differentiate_args")),
		ActivityDumpLocalStorageDirectory:     cfg.GetString(join(adNS, "local_storage", "output_directory")),
		ActivityDumpLocalStorageMaxDumpsCount: cfg.GetInt(join(adNS, "local_storage", "max_dumps_count")),
		ActivityDumpLocalStorageCompression:   cfg.GetBool(join(adNS, "local_storage", "compression")),
		ActivityDumpSyscallMonitorPeriod:      time.Duration(cfg.GetInt(join(adNS, "syscall_monitor", "period"))) * time.Second,
		ActivityDumpMaxDumpCountPerWorkload:   cfg.GetInt(join(adNS, "max_dump_count_per_workload")),

		// dynamic fields
		ActivityDumpMaxDumpSize: func() int {
			mds := cfg.GetInt(join(adNS, "max_dump_size"))
			if mds < MinMaxDumSize {
				mds = MinMaxDumSize
			}
			return mds * (1 << 10)
		},
	}

	if err := c.sanitize(); err != nil {
		return nil, err
	}
	return c, nil
}

// sanitize ensures that runtime_security_config.activity_dump is properly configured
func (c *Config) sanitize() error {
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

	if formats := coreconfig.SystemProbe.GetStringSlice(join(adNS, "local_storage", "formats")); len(formats) > 0 {
		var err error
		c.ActivityDumpLocalStorageFormats, err = ParseStorageFormats(formats)
		if err != nil {
			return fmt.Errorf("invalid value for runtime_security_config.activity_dump.local_storage.formats: %w", err)
		}
	}
	if formats := coreconfig.Datadog.GetStringSlice(join(adNS, "remote_storage", "formats")); len(formats) > 0 {
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
