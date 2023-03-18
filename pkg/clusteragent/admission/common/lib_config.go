// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// LibConfig holds the APM library configuration
type LibConfig struct {
	Language string `yaml:"library_language" json:"library_language"`
	Version  string `yaml:"library_version" json:"library_version"`

	ServiceName *string `yaml:"service_name,omitempty" json:"service_name,omitempty"`
	Env         *string `yaml:"env,omitempty" json:"env,omitempty"`

	Tracing        *bool `yaml:"tracing_enabled,omitempty" json:"tracing_enabled,omitempty"`
	LogInjection   *bool `yaml:"log_injection_enabled,omitempty" json:"log_injection_enabled,omitempty"`
	HealthMetrics  *bool `yaml:"health_metrics_enabled,omitempty" json:"health_metrics_enabled,omitempty"`
	RuntimeMetrics *bool `yaml:"runtime_metrics_enabled,omitempty" json:"runtime_metrics_enabled,omitempty"`

	TracingSamplingRate *float64 `yaml:"tracing_sampling_rate" json:"tracing_sampling_rate,omitempty"`
	TracingRateLimit    *int     `yaml:"tracing_rate_limit" json:"tracing_rate_limit,omitempty"`
	TracingTags         []string `yaml:"tracing_tags" json:"tracing_tags,omitempty"`

	TracingServiceMapping          []TracingServiceMapEntry `yaml:"tracing_service_mapping" json:"tracing_service_mapping,omitempty"`
	TracingAgentTimeout            *int                     `yaml:"tracing_agent_timeout" json:"tracing_agent_timeout,omitempty"`
	TracingHeaderTags              []TracingHeaderTagEntry  `yaml:"tracing_header_tags" json:"tracing_header_tags,omitempty"`
	TracingPartialFlushMinSpans    *int                     `yaml:"tracing_partial_flush_min_spans" json:"tracing_partial_flush_min_spans,omitempty"`
	TracingDebug                   *bool                    `yaml:"tracing_debug" json:"tracing_debug,omitempty"`
	TracingLogLevel                *string                  `yaml:"tracing_log_level" json:"tracing_log_level,omitempty"`
	TracingMethods                 []string                 `yaml:"tracing_methods" json:"tracing_methods,omitempty"`
	TracingPropagationStyleInject  []string                 `yaml:"tracing_propagation_style_inject" json:"tracing_propagation_style_inject,omitempty"`
	TracingPropagationStyleExtract []string                 `yaml:"tracing_propagation_style_extract" json:"tracing_propagation_style_extract,omitempty"`
}

// TracingServiceMapEntry holds service mapping config
type TracingServiceMapEntry struct {
	FromKey string `yaml:"from_key" json:"from_key"`
	ToName  string `yaml:"to_name" json:"to_name"`
}

// TracingHeaderTagEntry holds header tags config
type TracingHeaderTagEntry struct {
	Header  string `yaml:"header" json:"header"`
	TagName string `yaml:"tag_name" json:"tag_name"`
}

// ToEnvs converts the config fields into environment variables
func (lc LibConfig) ToEnvs() []corev1.EnvVar {
	var envs []corev1.EnvVar
	if lc.ServiceName != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_SERVICE",
			Value: *lc.ServiceName,
		})
	}
	if lc.Env != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_ENV",
			Value: *lc.Env,
		})
	}
	if val, defined := checkFormatVal(lc.Tracing); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_ENABLED",
			Value: val,
		})
	}
	if val, defined := checkFormatVal(lc.LogInjection); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_LOGS_INJECTION",
			Value: val,
		})
	}
	if val, defined := checkFormatVal(lc.HealthMetrics); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
			Value: val,
		})
	}
	if val, defined := checkFormatVal(lc.RuntimeMetrics); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_RUNTIME_METRICS_ENABLED",
			Value: val,
		})
	}
	if val, defined := checkFormatFloat(lc.TracingSamplingRate); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_SAMPLE_RATE",
			Value: val,
		})
	}
	if val, defined := checkFormatVal(lc.TracingRateLimit); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_RATE_LIMIT",
			Value: val,
		})
	}
	if lc.TracingTags != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TAGS",
			Value: strings.Join(lc.TracingTags, ","),
		})
	}
	if lc.TracingServiceMapping != nil {
		pairs := make([]string, 0, len(lc.TracingServiceMapping))
		for _, m := range lc.TracingServiceMapping {
			pairs = append(pairs, fmt.Sprintf("%s:%s", m.FromKey, m.ToName))
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_SERVICE_MAPPING",
			Value: strings.Join(pairs, ", "),
		})
	}
	if val, defined := checkFormatVal(lc.TracingAgentTimeout); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_AGENT_TIMEOUT",
			Value: val,
		})
	}
	if lc.TracingHeaderTags != nil {
		pairs := make([]string, 0, len(lc.TracingHeaderTags))
		for _, m := range lc.TracingHeaderTags {
			pairs = append(pairs, fmt.Sprintf("%s:%s", m.Header, m.TagName))
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_HEADER_TAGS",
			Value: strings.Join(pairs, ", "),
		})
	}
	if val, defined := checkFormatVal(lc.TracingPartialFlushMinSpans); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_PARTIAL_FLUSH_MIN_SPANS",
			Value: val,
		})
	}
	if val, defined := checkFormatVal(lc.TracingDebug); defined {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_DEBUG",
			Value: val,
		})
	}
	if lc.TracingLogLevel != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_LOG_LEVEL",
			Value: *lc.TracingLogLevel,
		})
	}
	if lc.TracingMethods != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_TRACE_METHODS",
			Value: strings.Join(lc.TracingMethods, ";"),
		})
	}
	if lc.TracingPropagationStyleInject != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_PROPAGATION_STYLE_INJECT",
			Value: strings.Join(lc.TracingPropagationStyleInject, ","),
		})
	}
	if lc.TracingPropagationStyleExtract != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_PROPAGATION_STYLE_EXTRACT",
			Value: strings.Join(lc.TracingPropagationStyleExtract, ","),
		})
	}
	return envs
}

func checkFormatVal[T int | bool](val *T) (string, bool) {
	if val == nil {
		return "", false
	}
	return fmt.Sprintf("%v", *val), true
}

func checkFormatFloat(val *float64) (string, bool) {
	if val == nil {
		return "", false
	}
	return fmt.Sprintf("%.2f", *val), true
}
