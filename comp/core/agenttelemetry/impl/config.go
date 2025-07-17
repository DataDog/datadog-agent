// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	defaultSheduleStartAfter = 30  // 30 seconds
	defaultShedulePeriod     = 900 // 15 minutes
)

// Config is the top-level config for agent telemetry
type Config struct {
	Enabled  bool       `yaml:"enabled"`
	Profiles []*Profile `yaml:"profiles"`

	// config-wide and "compiled" fields
	schedule map[Schedule][]*Profile
	events   map[string]*Event

	StartupTraceSampling float64 `yaml:"startup_trace_sampling"`
}

// Profile is a single agent telemetry profile
type Profile struct {
	// parsed
	Name     string             `yaml:"name"`
	Metric   *AgentMetricConfig `yaml:"metric,omitempty"`
	Schedule *Schedule          `yaml:"schedule,omitempty"`
	Events   []*Event           `yaml:"events"`

	// compiled
	metricsMap        map[string]*MetricConfig
	excludeZeroMetric bool
	excludeTagsMap    map[string]any
}

// AgentMetricConfig specifies agent telemetry metrics payloads to be generated and emitted
type AgentMetricConfig struct {
	Exclude *ExcludeMetricConfig `yaml:"exclude,omitempty"`
	Metrics []MetricConfig       `yaml:"metrics,omitempty"`
}

// ExcludeMetricConfig specifies agent telemetry metrics to be excluded from the agent telemetry
type ExcludeMetricConfig struct {
	ZeroMetric *bool    `yaml:"zero_metric,omitempty"`
	Tags       []string `yaml:"tags,omitempty"`
}

// MetricConfig is a list of metric selecting subset of telemetry.Gather() metrics to be included in agent
type MetricConfig struct {
	Name           string   `yaml:"name"` // required
	AggregateTags  []string `yaml:"aggregate_tags,omitempty"`
	AggregateTotal bool     `yaml:"aggregate_total"`

	// compiled
	aggregateTagsExists bool
	aggregateTagsMap    map[string]any
}

// Schedule is a schedule for agent telemetry payloads to be generated and emitted
type Schedule struct {
	// parsed
	StartAfter uint `yaml:"start_after"`
	Iterations uint `yaml:"iterations"`
	Period     uint `yaml:"period"`
}

// Event is a payload sent by Agent Telemetry component client
type Event struct {
	Name        string `yaml:"name"`         // required
	RequestType string `yaml:"request_type"` // required
	PayloadKey  string `yaml:"payload_key"`  // required
	Message     string `yaml:"message"`      // required
}

// profiles[].metric.metrics (optional)
// --------------------------
// When included, agent telemetry metrics payloads will be generated and emitted.
//
// profiles[].metric.exclude (optional)
// -------------------------------------
// When included, specifies agent telemetry metrics to be excluded from the agent telemetry.
//
// profiles[].metric.exclude.zero_metric (optional)
// ------------------------------------------------
// When included, specifies whether metrics with zero value should be excluded from the
// agent telemetry metrics payloads. If not specified, default value of `false` will be used.
//
// profiles[].metric.exclude.tags[] (optional)
// ------------------------------------------
// When included, specifies metrics with its tags and optionally values to beexcluded from
// the agent telemetry metrics payloads. A tag value can be specified with or without values.
// If specified without value, all metrics with the tag will be excluded. If specified with
// value, only metrics with the matching tag and the value will be excluded.
//
// profiles[].metric.metrics[].name (required)
// -------------------------------------------
// The full metric name is formatted as "<metric group>.<metric name>". In telemetry.NewGauge()
// and similar APIs, "metric group" corresponds to the "subsystem" parameter, and "metric name"
// corresponds to the "name" parameter. Do not use the "Options.NoDoubleUnderscoreSep" option
// in these APIs, as it is not supported in agent telemetry.
//
// profiles[].metric.metrics[].aggregate_tags (optional)
// -----------------------------------------------------
// List of tags to be used for metric aggregation. If not specified, or [] is specified,
// metric will be aggregated without any tags. If specified, metric will be aggregated using
// the specified tags. Unspecified tags
//   * will not be used and effectively will be removed from the metric's JSON object
//   * their timeseries value will be summed up according to the remaining metric tags
//   * in case if no tags a specified, all timeseries will be summed up and no tags will be
//     reported in the metric's JSON object
//   * in case none of the tags matches to the aggregateTags time series will be fremoved
//     from the metric's JSON object
// The primary goal of such aggregation is not actually to reduce the number of timeseries
// and the amount of data to be sent to the backend, although it is welcome side-effect,
// but to make sure that no privacy leak will happen by accident, by enforcing requirement
// for explicit tag specification.
//
// profiles[].metric.metrics[].aggregate_total (optional)
// -----------------------------------------------------
// When included, specifies whether the metric should be aggregated as a total. A
// special tag "total" will be added to the metric's JSON object (accordingly "total is
// reserved tag"). If not specified, specified, default value of `false` will be used.
// It is useful only if "aggregate_tags" is also specified and will be ignored otherwise.
//
// profiles[].schedule (optional)
// --------------------------------
// Specified when agent telemetry payloads to be generated and emitted. If not specified,
// configured payloads willbe generated and emitted on the following schedule (the details
// are described in the comments below.
//
//    (legend - 300s=5m, 900s=15m, 1800s=30m, 3600s=1h, 14400s=4h, 86400s=1d)
//
//        schedule:
//          start_after: 30
//          iterations: 0
//          period: 900
//
// profiles[].schedule.start_after (optional)
// ------------------------------------------
// Number of seconds to wait after agent start before starting telemetry collection for the
// profile. If not specified, default values are specified above.
//
// profiles[].schedule.iterations (optional)
// -----------------------------------------
// Number of telemetry collection iterations to perform for the profile. To indicate infinite
// number of iterations, use 0. If not specified, default value of 0 will be used.
//
// profiles[].schedule.period (optional)
// -------------------------------------
// Number of seconds to wait between telemetry collection iteration for the profile. If not
// specified, default values are specified above.
//
// profiles[].events (optional)
// -------------------------------------
// List of registered events an agent telemetry client can send
//
// profiles[].events[].name (required)
// -------------------------------------
// The name of the event to find corresponding request_type, payload_key and message values
//
// profiles[].events[].request_type (required)
// -------------------------------------
// The value is required and used in the corresponding payload to identify the event
//
// profiles[].events[].payload_key (required)
// -------------------------------------
// The value is required and used in the corresponding payload as a root of the event payload
//
// profiles[].events[].message (required)
// -------------------------------------
// The value is required and used in the corresponding payload

// ----------------------------------------------------------------------------------
//
// Default agent telemetry profiles config if not specified in the agent config file.
// Note: If "aggregate_tags" are not specified, metric will be aggregated without any tags.
var defaultProfiles = `
  profiles:
  - name: checks
    metric:
      metrics:
        - name: checks.execution_time
          aggregate_tags:
            - check_name
            - check_loader
        - name: pymem.inuse
    schedule:
      start_after: 30
      iterations: 0
      period: 900
  - name: logs-and-metrics
    metric:
      metrics:
        - name: dogstatsd.udp_packets_bytes
        - name: dogstatsd.uds_packets_bytes
        - name: logs.bytes_missed
        - name: logs.bytes_sent
        - name: logs.decoded
        - name: logs.dropped
        - name: logs.encoded_bytes_sent
          aggregate_tags:
            - compression_kind
        - name: logs.sender_latency
        - name: logs.truncated
          aggregate_tags:
            - service
            - source
        - name: logs.auto_multi_line_aggregator_flush
          aggregate_tags:
            - truncated
            - line_type
        - name: logs_destination.destination_workers
        - name: point.sent
        - name: point.dropped
        - name: transactions.input_count
        - name: transactions.requeued
        - name: transactions.retries
        - name: transactions.http_errors
          aggregate_tags:
            - code
            - endpoint
    schedule:
      start_after: 30
      iterations: 0
      period: 900
  - name: database
    metric:
      exclude:
        zero_metric: true
      metrics:
        - name: oracle.activity_samples_count
        - name: oracle.activity_latency
        - name: oracle.statement_metrics
        - name: oracle.statement_plan_errors
        - name: postgres.collect_activity_snapshot_ms
        - name: postgres.collect_relations_autodiscovery_ms
        - name: postgres.collect_statement_samples_ms
        - name: postgres.collect_statement_samples_count
        - name: postgres.collect_stat_autodiscovery_ms
        - name: postgres.get_active_connections_ms
        - name: postgres.get_active_connections_count
        - name: postgres.get_new_pg_stat_activity_count
        - name: postgres.get_new_pg_stat_activity_ms
        - name: postgres.schema_tables_elapsed_ms
        - name: postgres.schema_tables_count
    schedule:
      start_after: 30
      iterations: 0
      period: 900
  - name: api
    metric:
      exclude:
        zero_metric: true
      metrics:
        - name: api_server.request_duration_seconds
          aggregate_tags:
            - servername
            - status_code
            - method
            - path
            - auth
    schedule:
      start_after: 600
      iterations: 0
      period: 14400
  - name: ondemand
    events:
      - name: agentbsod
        request_type: agent-bsod
        payload_key: agent_bsod
        message: 'Agent BSOD'
  - name: service-discovery
    metric:
      metrics:
        - name: service_discovery.discovered_services
    schedule:
      start_after: 30
      iterations: 0
      period: 900
  - name: runtime-started
    metric:
      exclude:
        zero_metric: true
      metrics:
        - name: runtime.started
    schedule:
      start_after: 5
      iterations: 1
  - name: runtime-running
    metric:
      exclude:
        zero_metric: true
      metrics:
        - name: runtime.running
`

func compileMetricsExclude(p *Profile) error {
	if p.Metric.Exclude == nil {
		return nil
	}

	if p.Metric.Exclude.ZeroMetric == nil && len(p.Metric.Exclude.Tags) == 0 {
		return fmt.Errorf("profile '%s' requires either 'metric.exclude.zero_metric' or 'metric.exclude.tags' attribute to be specified", p.Name)
	}

	// Exclude zero metric (optional with default "false")
	if p.Metric.Exclude.ZeroMetric != nil {
		p.excludeZeroMetric = *p.Metric.Exclude.ZeroMetric
	} else {
		p.excludeZeroMetric = false
	}

	// Exclude tags (optional)
	p.excludeTagsMap = make(map[string]any)
	for _, t := range p.Metric.Exclude.Tags {
		tv := strings.SplitN(t, ":", 2)
		// Setup for 2 cases - exclude a tag with any value or only with a specific value
		if len(tv) == 1 {
			// previous value does not matter, we do not care about tag values anymore
			p.excludeTagsMap[tv[0]] = struct{}{}
			continue
		}

		// let's see if the tag is already in the map ...
		if v, ok := p.excludeTagsMap[tv[0]]; ok {
			// ... and it value-less meaning any value is excluded
			if _, ok := v.(struct{}); !ok {
				(v.(map[string]struct{})[tv[1]]) = struct{}{}
			}
		} else {
			// If the tag and value is not in the map yet, let's add it
			vals := make(map[string]struct{})
			vals[tv[1]] = struct{}{}
			p.excludeTagsMap[tv[0]] = vals
		}
	}

	return nil
}

func compileMetric(p *Profile, m *MetricConfig) error {
	// Validate name (required)
	if len(m.Name) == 0 {
		return fmt.Errorf("profile '%s' requires 'metrics[].name' attribute to be specified", p.Name)
	}
	// Split metric name into metric group and metric name to convert it to Prometheus metric name
	names := strings.Split(m.Name, ".")
	if len(names) != 2 {
		return fmt.Errorf("profile '%s' 'metrics[].name' '(%s)' attribute should have two elements separated by '.'", p.Name, m.Name)
	}

	// Converts a Datadog metric name to a Prometheus metric name for quicker matching. Prometheus metrics
	// (from the "telemetry" package) must be declared without setting Options.NoDoubleUnderscoreSep to true,
	// ensuring the full metric name includes double underscores ("__"); otherwise, matching will fail.
	promName := fmt.Sprintf("%s__%s", names[0], names[1])
	p.metricsMap[promName] = m

	// Compile aggregate tags (optional)
	if len(m.AggregateTags) == 0 {
		m.aggregateTagsExists = false
	} else {
		m.aggregateTagsExists = true
		m.aggregateTagsMap = make(map[string]any)
		for _, t := range m.AggregateTags {
			m.aggregateTagsMap[t] = struct{}{}
		}
	}

	return nil
}

// Validate profiles
func validateProfiles(cfg *Config) error {
	for i, p := range cfg.Profiles {
		if len(p.Name) == 0 {
			return fmt.Errorf("profile requires 'name' attribute to be specified. Profile index: %d", i)
		}
	}

	return nil
}

func compileMetrics(cfg *Config) error {
	for _, p := range cfg.Profiles {
		// No metric section - nothing to do
		if p.Metric == nil || len(p.Metric.Metrics) == 0 {
			continue
		}

		if err := compileMetricsExclude(p); err != nil {
			return err
		}

		// Compile metrics themselves
		p.metricsMap = make(map[string]*MetricConfig)
		for i := 0; i < len(p.Metric.Metrics); i++ {
			if err := compileMetric(p, &p.Metric.Metrics[i]); err != nil {
				return err
			}
		}
	}

	return nil
}

// Compile schedules
func compileSchedules(cfg *Config) error {
	cfg.schedule = make(map[Schedule][]*Profile)

	for i := 0; i < len(cfg.Profiles); i++ {
		p := cfg.Profiles[i]

		// No metric section - schedule is not needed
		if p.Metric == nil || len(p.Metric.Metrics) == 0 {
			continue
		}

		// Setup default schedule if it is not specified partially or at all
		if p.Schedule == nil {
			p.Schedule = &Schedule{
				StartAfter: defaultSheduleStartAfter,
				Iterations: 0,
				Period:     defaultShedulePeriod,
			}
		} else {
			// Validate StartAfter (optional)
			if p.Schedule.StartAfter == 0 {
				p.Schedule.StartAfter = defaultSheduleStartAfter
			}

			// Validate Period (optional)
			if p.Schedule.Period == 0 {
				p.Schedule.Period = defaultShedulePeriod
			}
		}

		// Aggregate schedules (one schedule correspond to one or more profiles)
		if pp, ok := cfg.schedule[*p.Schedule]; ok {
			pp = append(pp, p)
			cfg.schedule[*p.Schedule] = pp
		} else {
			pp = []*Profile{p}
			cfg.schedule[*p.Schedule] = pp
		}
	}

	return nil
}

func compileEvents(cfg *Config) error {
	cfg.events = make(map[string]*Event)
	for _, p := range cfg.Profiles {
		if p.Events != nil {
			for _, e := range p.Events {
				cfg.events[e.Name] = e
			}
		}
	}

	return nil
}

// Compile agent telemetry config
func compileConfig(cfg *Config) error {
	if err := validateProfiles(cfg); err != nil {
		return err
	}

	if err := compileMetrics(cfg); err != nil {
		return err
	}

	if err := compileSchedules(cfg); err != nil {
		return err
	}

	if err := compileEvents(cfg); err != nil {
		return err
	}

	return nil
}

// Parse agent telemetry config
func parseConfig(cfg config.Component) (*Config, error) {
	// Is it enabled?
	if !pkgconfigsetup.IsAgentTelemetryEnabled(cfg) {
		return &Config{
			Enabled: false,
		}, nil
	}

	var atCfg Config

	// Parse agent telemetry config
	atCfgMap := cfg.GetStringMap("agent_telemetry")
	if len(atCfgMap) > 0 {
		// Reconvert to string and back to object.
		// Config.UnmarshalKey() is better but it did not work in some cases
		atCfgBytes, err := yaml.Marshal(atCfgMap)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(atCfgBytes, &atCfg)
		if err != nil {
			return nil, err
		}
	}

	// Add default profiles if not specified
	if len(atCfg.Profiles) == 0 {
		err := yaml.Unmarshal([]byte(defaultProfiles), &atCfg)
		if err != nil {
			return nil, err
		}

		atCfg.Enabled = true
	}

	// Compile agent telemetry config
	err := compileConfig(&atCfg)
	if err != nil {
		return nil, err
	}

	// Successful parsing
	return &atCfg, nil
}
