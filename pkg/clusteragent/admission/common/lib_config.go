// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package common

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// LibConfig holds the APM library configuration
type LibConfig struct {
	Version         int    `yaml:"version,omitempty" json:"version,omitempty"` // config schema version, not config version
	ServiceLanguage string `yaml:"service_language,omitempty" json:"service_language,omitempty"`

	Tracing             *bool    `yaml:"tracing_enabled,omitempty" json:"tracing_enabled,omitempty"`
	LogInjection        *bool    `yaml:"log_injection_enabled,omitempty" json:"log_injection_enabled,omitempty"`
	HealthMetrics       *bool    `yaml:"health_metrics_enabled,omitempty" json:"health_metrics_enabled,omitempty"`
	RuntimeMetrics      *bool    `yaml:"runtime_metrics_enabled,omitempty" json:"runtime_metrics_enabled,omitempty"`
	TracingSamplingRate *float64 `yaml:"tracing_sampling_rate,omitempty" json:"tracing_sampling_rate,omitempty"`
	TracingRateLimit    *int     `yaml:"tracing_rate_limit,omitempty" json:"tracing_rate_limit,omitempty"`
	TracingTags         []string `yaml:"tracing_tags,omitempty" json:"tracing_tags,omitempty"`
}

// ToEnvs converts the config fields into environment variables
func (lc LibConfig) ToEnvs() []corev1.EnvVar {
	var envs []corev1.EnvVar
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
