// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"strings"
)

const (
	// apmBasicTargetName is the name of the target injecting dd-trace-c in APM Basic mode.
	apmBasicTargetName = "apm-basic"
	// apmBasicEnabledLabelKey is the pod label that opts a workload into APM Basic.
	apmBasicEnabledLabelKey = "admission.datadoghq.com/apm-basic.enabled"
)

// apmBasicHookModules are the dd-trace-c hook modules that observe HTTP(S) and database traffic. "all" is not used
// because it also selects the GPU module.
var apmBasicHookModules = []string{"socket", "openssl", "postgres", "mysql", "sqlite", "mongodb", "oracle"}

// APMBasicConfig configures the target injecting dd-trace-c in APM Basic mode, which reports RED metrics without
// sending traces.
type APMBasicConfig struct {
	// Enabled indicates whether dd-trace-c should be injected into workloads opted into APM Basic. Full config key:
	// apm_config.instrumentation.apm_basic.enabled
	Enabled bool `mapstructure:"enabled" json:"enabled"`
	// TracerVersions maps a tracer language to the tracer version to inject. Full config key:
	// apm_config.instrumentation.apm_basic.tracer_versions
	TracerVersions map[string]string `mapstructure:"tracer_versions" json:"tracer_versions"`
}

// newAPMBasicTarget builds the target injecting dd-trace-c in APM Basic mode, or nil when APM Basic is disabled.
//
// DD_INJECT_NATIVE=fallback loads dd-trace-c only into processes where the injector did not inject a language SDK,
// so a process is never observed by two tracers.
func newAPMBasicTarget(cfg APMBasicConfig) *Target {
	if !cfg.Enabled {
		return nil
	}

	return &Target{
		Name: apmBasicTargetName,
		PodSelector: &PodSelector{
			MatchLabels: map[string]string{apmBasicEnabledLabelKey: "true"},
		},
		TracerVersions: cfg.TracerVersions,
		TracerConfigs: []TracerConfig{
			{Name: "DD_INJECT_NATIVE", Value: "fallback"},
			{Name: "DD_TRACE_HOOK_MODULES", Value: strings.Join(apmBasicHookModules, ",")},
			{Name: "DD_APM_BASIC_ENABLED", Value: "true"},
		},
	}
}
