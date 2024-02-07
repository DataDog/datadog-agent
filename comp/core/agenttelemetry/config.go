// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"gopkg.in/yaml.v2"
)

const (
	defaultSheduleStartAfter = 30  // 30 seconds
	defaultShedulePeriod     = 900 // 15 minutes
)

// Config is the top-level config for agent telemetry
type Config struct {
	Enabled  bool      `yaml:"enabled"`
	Profiles []Profile `yaml:"profiles"`

	// compiled
	schedule map[Schedule][]*Profile
}

// Profile is a single agent telemetry profile
type Profile struct {
	// parsed
	Name     string             `yaml:"name"`
	Metric   *AgentMetricConfig `yaml:"metric,omitempty"`
	Status   *AgentStatusConfig `yaml:"status"`
	Schedule *Schedule          `yaml:"schedule"`

	// compiled
	statusExtraBuilder []jBuilder
	metricsMap         map[string]*MetricConfig
	excludeZeroMetric  bool
	excludeTags        map[string]any
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
	Name string `yaml:"name"` // required
}

// AgentStatusConfig is a single agent telemetry status payload
type AgentStatusConfig struct {
	Template string            `yaml:"template"`
	Extra    map[string]string `yaml:"extra,omitempty"`
}

// Schedule is a schedule for agent telemetry payloads to be generated and emitted
type Schedule struct {
	// parsed
	StartAfter uint `yaml:"start_after"`
	Iterations uint `yaml:"iterations"`
	Period     uint `yaml:"period"`
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
// Metric's full name typically in the form of "<metric group>.<metric name>".
// It is required parameter to avoid emitting all available metrics unintentionally.
//
// profiles[].status (optional)
// --------------------------------
// When included, agent telemetry status payloads will be generated and emitted.
//
// profiles[].status.template (optional)
// --------------------------------------
// Name of agent status JSON rendering template which generates agent telemetry status
// payload. Used as a suffix to
//   "pkg\status\render\templates\agent-telemetry-<template name>.tmpl" file path. Currently
// two templates ("basic" and "none") are supported. When "none" template is used for a
// profile, agent telemetry status payload will not be generated for that profile (unless
// "extra" configuration is specified).
//
// profiles[].status.extra (optional)
// ----------------------------------
// Map of extra telemetry JSON objects/attributes selected or calculated from the main Agent
// Status JSON object. Each map entry specifies a single JSON object and its single
// attribute to be added to the agent status telemetry payload.
//
//    key - "." separated elements specifying "target" JSON path to the additional JSON object
//      to be added to the agent status telemetry payload. The rightmost component is the
//      object's attribute name.
//
//    value - JQ expression to be applied to the full agent status JSON to compute a value for
//      for the specified in the key agent telemetry status JSON attribute. For privacy and
//      security reasons, claculated value can  only be int, float or bool. Strings will be
//      allowed as exceptions provided they had been approved.
//
//       Example. If agent status JSON is ...
//           {
//             "runnerStats": {
//               "Workers": {
//                 "Count": 2,
//                 "Instances": {
//                   "worker_1": {
//                     "Utilization": 0.05
//                   },
//                   "worker_2": {
//                     "Utilization": 0.17
//                   }
//                 }
//               }
//             }
//           }
//
//       ... with "extra" like this ...
//           ...
//           extra:
//             workers.count: '.runnerStats.Workers.Count'
//             workers.utilization: '[.runnerStats.Workers | .. | objects | .Utilization] | add'
//
//       ... the following JSON object will be added to the agent telemetry statys payload ...
//           "workers": {
//             "count": 2,
//             "utilization": 0.22
//           }
//
// profiles[].schedule (optional)
// --------------------------------
// Specified when agent telemetry payloads to be generated and emitted. If not specified,
// configured payloads willbe generated and emitted on the following schedule (the details
// are described in the comments below.
//
//    (legend - 300s=5m, 900s=15m, 1800s=30m, 3600s=1h, 86400s=1d)
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

// ----------------------------------------------------------------------------------
//
// Default agent telemetry profiles config if not specified in the agent config file.
var defaultProfiles = `
  profiles:
  - name: core-metrics
    metric:
      exclude:
        zero_metric: true
        tags:
          - "check_name:cpu"
          - "check_name:memory"
          - "check_name:uptime"
          - "check_name:network"
          - "check_name:io"
          - "check_name:file_handle"
      metrics:  
        - name: checks.runs
        - name: checks.execution_time
        - name: checks.warnings
        - name: checks.metrics_samples
        - name: checks.events
        - name: logs.decoded
        - name: logs.processed
        - name: logs.sent
        - name: logs.network_errors
        - name: logs.dropped
        - name: logs.sender_latency
        - name: logs.destination_http_resp
        - name: transactions.input_count
        - name: transactions.requeued
        - name: transactions.retries
        - name: dogstatsd.udp_packets
        - name: dogstatsd.uds_packets
        - name: dogstatsd.uds_origin_detection_error
        - name: dogstatsd.uds_connections
    schedule:
      start_after: 30
      iterations: 0
      period: 900
  - name: core-status
    status:
      template: basic
      extra:
        runner.workers.count: '.runnerStats.Workers.Count'
        runner.workers.utilization: '[.runnerStats.Workers | .. | objects | .Utilization] | add'
    schedule:
      start_after: 30
      iterations: 0
      period: 900
`

func compileAgentTelemetryProfile(p *Profile) error {
	var err error

	// Profile requires name
	if len(p.Name) == 0 {
		return fmt.Errorf("profile requires 'name' attribute to be specified")
	}

	// -----------------------------------
	// "Compile" metric section (optional)
	//
	if p.Metric != nil {
		// Exclude (optional)
		if p.Metric.Exclude != nil {
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
			p.excludeTags = make(map[string]any)
			for _, t := range p.Metric.Exclude.Tags {
				tv := strings.SplitN(t, ":", 2)
				// Setup for 2 cases - exclude a tag with any value or only with a specific value
				if len(tv) == 1 {
					// previous value does not matter, we do not care about tag values anymore
					p.excludeTags[tv[0]] = struct{}{}
					continue
				}

				// let's see if the tag is already in the map ...
				if v, ok := p.excludeTags[tv[0]]; ok {
					// ... and it value-less meaning any value is excluded
					if _, ok := v.(struct{}); !ok {
						(v.(map[string]struct{})[tv[1]]) = struct{}{}
					}
				} else {
					// If the tag and value is not in the map yet, let's add it
					vals := make(map[string]struct{})
					vals[tv[1]] = struct{}{}
					p.excludeTags[tv[0]] = vals
				}
			}
		}

		// Compile metrics themselves
		p.metricsMap = make(map[string]*MetricConfig)
		for i := 0; i < len(p.Metric.Metrics); i++ {
			metric := &p.Metric.Metrics[i]

			// Validate name (required)
			if len(metric.Name) == 0 {
				return fmt.Errorf("profile '%s' requires 'metrics[].name' attribute to be specified", p.Name)
			}
			// Split metric name into metric group and metric name to convert it to Prometheus metric name
			metricNames := strings.Split(metric.Name, ".")
			if len(metricNames) != 2 {
				return fmt.Errorf("profile '%s' 'metrics[].name' '(%s)' attribute should have two elements separated by '.'", p.Name, metric.Name)
			}

			// Convert Datadog metric name to Prometheus metric name (used for quick(er) matching)
			promName := fmt.Sprintf("%s__%s", metricNames[0], metricNames[1])
			p.metricsMap[promName] = metric
		}
	}

	// -----------------------------------
	// "Compile" status section (optional)
	//
	if p.Status != nil {
		// Validate template (optional with default "basic". "none" is also supported)
		if len(p.Status.Template) == 0 {
			p.Status.Template = "basic"
		} else if p.Status.Template != "basic" && p.Status.Template != "none" {
			return fmt.Errorf("profile '%s' template attribute can have 'basic' or 'none' value but %s is provided", p.Name, p.Status.Template)
		}

		// Compile status extra
		p.statusExtraBuilder, err = compileJBuilders(p.Status.Extra)
		if err != nil {
			return fmt.Errorf("failed to compile 'extra' attribute for profile '%s'. Error: %w", p.Name, err)
		}
	}

	return nil
}

func compileAgentTelemetrySchedules(cfg *Config) error {
	cfg.schedule = make(map[Schedule][]*Profile)

	for i := 0; i < len(cfg.Profiles); i++ {
		p := &cfg.Profiles[i]

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

func compileConfig(cfg *Config) error {
	for i := 0; i < len(cfg.Profiles); i++ {
		err := compileAgentTelemetryProfile(&cfg.Profiles[i])
		if err != nil {
			return err
		}
	}

	if err := compileAgentTelemetrySchedules(cfg); err != nil {
		return err
	}

	return nil
}

// Parse agent telemetry config
func parseConfig(cfg config.Component) (*Config, error) {
	// Is it enabled?
	if !cfg.GetBool("agent_telemetry.enabled") {
		return &Config{
			Enabled: false,
		}, nil
	}

	// Parse agent telemetry config
	var atCfg Config
	err := cfg.UnmarshalKey("agent_telemetry", &atCfg)
	if err != nil {
		return nil, err
	}

	// Add default profiles if not specified
	if len(atCfg.Profiles) == 0 {
		err = yaml.Unmarshal([]byte(defaultProfiles), &atCfg)
		if err != nil {
			return nil, err
		}

		atCfg.Enabled = true
	}

	// Compile agent telemetry config
	err = compileConfig(&atCfg)
	if err != nil {
		return nil, err
	}

	// Successful parsing
	return &atCfg, nil
}
