// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpuenvironment

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// SystemProbeDataIssueName is the human-readable issue name for GPU data
	// that the core agent expects from system-probe but is not receiving.
	SystemProbeDataIssueName = "GPU System-Probe Data Unavailable"
	// SystemProbeDataIssueType is the snake_case type key: SystemProbeDataIssueName
	// lowercased with spaces replaced by underscores.
	SystemProbeDataIssueType = "gpu_system-probe_data_unavailable"
	// SystemProbeDataIssueID is the issue ID for this issue. There is at most one instance per agent.
	SystemProbeDataIssueID = "gpu-system-probe-data-unavailable"

	sysprobeDataIssueName = SystemProbeDataIssueName
	sysprobeDataIssueType = SystemProbeDataIssueType
	sysprobeDataImpactMsg = "GPU metrics that depend on system-probe are not being collected"

	maxSystemProbeErrorLength = 512
)

// System-probe data sources consumed by the GPU check.
const (
	// SystemProbeSourceEBPF is the eBPF-based process monitoring data (/gpu/check).
	SystemProbeSourceEBPF = "ebpf"
	// SystemProbeSourcePRM is the privileged NVLink PRM counters (/gpu/prm-metrics).
	SystemProbeSourcePRM = "prm"
	// SystemProbeSourceDriverEvents is the NVIDIA driver events read from the kernel log (/gpu/driver-events).
	SystemProbeSourceDriverEvents = "driver_events"
)

// Error classes for a failing system-probe data source.
const (
	// SystemProbeErrorUnreachable means the system-probe socket could not be reached.
	SystemProbeErrorUnreachable = "system_probe_unreachable"
	// SystemProbeErrorModuleNotLoaded means system-probe answered, but the GPU monitoring module is not loaded.
	SystemProbeErrorModuleNotLoaded = "module_not_loaded"
	// SystemProbeErrorSourceDisabled means the GPU monitoring module is loaded, but the data source is not running in it.
	SystemProbeErrorSourceDisabled = "source_disabled"
	// SystemProbeErrorTimeout means requests to system-probe timed out.
	SystemProbeErrorTimeout = "timeout"
	// SystemProbeErrorOther is any other error.
	SystemProbeErrorOther = "other"
)

// SystemProbeSourceFailure describes a system-probe data source that is failing.
type SystemProbeSourceFailure struct {
	Source     string
	ErrorClass string
	Error      string
}

type systemProbeSourceInfo struct {
	description string
	configKey   string
}

var systemProbeSources = map[string]systemProbeSourceInfo{
	SystemProbeSourceEBPF: {
		description: "process-level GPU metrics (gpu.process.*)",
		configKey:   "gpu_monitoring.enable_ebpf_probes",
	},
	SystemProbeSourcePRM: {
		description: "NVLink link error metrics from PRM registers",
		configKey:   "gpu_monitoring.prm_endpoint_enabled",
	},
	SystemProbeSourceDriverEvents: {
		description: "XID errors reported in the kernel log (gpu.errors.xid)",
		configKey:   "gpu_monitoring.driver_events_enabled",
	},
}

// SystemProbeDataIssue provides the issue template for GPU data missing from system-probe.
type SystemProbeDataIssue struct{}

// NewSystemProbeDataIssue creates a new system-probe data issue template.
func NewSystemProbeDataIssue() *SystemProbeDataIssue {
	return &SystemProbeDataIssue{}
}

// BuildIssue creates the issue for the given failing sources. Failures for unknown sources are rejected.
func (t *SystemProbeDataIssue) BuildIssue(failures []SystemProbeSourceFailure) (*healthplatform.Issue, error) {
	if len(failures) == 0 {
		return nil, errors.New("no failing system-probe sources")
	}

	failures = slices.Clone(failures)
	slices.SortFunc(failures, func(a, b SystemProbeSourceFailure) int { return strings.Compare(a.Source, b.Source) })

	descriptions := make([]string, 0, len(failures))
	sourceNames := make([]string, 0, len(failures))
	affected := make([]any, 0, len(failures))
	errorClasses := make(map[string]struct{})
	for _, f := range failures {
		info, ok := systemProbeSources[f.Source]
		if !ok {
			return nil, fmt.Errorf("unknown system-probe source: %s", f.Source)
		}
		errorClass := f.ErrorClass
		if errorClass == "" {
			errorClass = SystemProbeErrorOther
		}
		errorClasses[errorClass] = struct{}{}

		descriptions = append(descriptions, info.description)
		sourceNames = append(sourceNames, f.Source)
		affected = append(affected, map[string]any{
			"source":      f.Source,
			"description": info.description,
			"config_key":  info.configKey,
			"error_class": errorClass,
			"error":       truncate(f.Error, maxSystemProbeErrorLength),
		})
	}

	extra, err := structpb.NewStruct(map[string]any{
		"affected_sources": affected,
		"impact":           sysprobeDataImpactMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   sysprobeDataIssueName,
		IssueType:   sysprobeDataIssueType,
		Title:       "GPU monitoring is not receiving data from system-probe for: " + strings.Join(sourceNames, ", "),
		Description: "The GPU check expects data from the system-probe GPU monitoring module but cannot get it. Missing data: " + strings.Join(descriptions, "; ") + ".",
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify that system-probe is running with the GPU monitoring module enabled and reachable from the Agent.",
			Steps:   systemProbeDataSteps(errorClasses),
		},
		Tags: []string{"gpu", "system-probe"},
	}, nil
}

func systemProbeDataSteps(errorClasses map[string]struct{}) []*healthplatform.RemediationStep {
	var texts []string
	if _, ok := errorClasses[SystemProbeErrorUnreachable]; ok {
		texts = append(texts, "Verify that system-probe is running (in Kubernetes, the system-probe container of the Agent pod) and that its socket (system_probe_config.sysprobe_socket) is shared with the Agent")
	}
	if _, ok := errorClasses[SystemProbeErrorModuleNotLoaded]; ok {
		texts = append(texts,
			"Verify that gpu_monitoring.enabled is set to true in system-probe.yaml",
			"Check the system-probe logs for 'error creating module gpu'",
		)
	}
	if _, ok := errorClasses[SystemProbeErrorSourceDisabled]; ok {
		texts = append(texts,
			"Verify that the config_key of each affected source has the same value for the Agent and system-probe, including DD_ environment variables set on only one of the containers",
			"Check the system-probe logs for GPU monitoring startup errors, such as an unsupported kernel, eBPF compilation disabled, or NVML or /dev/kmsg unavailable",
		)
	}
	if _, ok := errorClasses[SystemProbeErrorTimeout]; ok {
		texts = append(texts, "If system-probe is responding slowly, increase gpu.sp_process_metrics_request_timeout in datadog.yaml")
	}
	if _, ok := errorClasses[SystemProbeErrorOther]; ok {
		texts = append(texts, "Check the Agent logs for GPU check errors mentioning system-probe")
	}
	texts = append(texts, "Review GPU monitoring setup: "+gpuDocsURL)

	steps := make([]*healthplatform.RemediationStep, 0, len(texts))
	for i, text := range texts {
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(i + 1), Text: text})
	}
	return steps
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
