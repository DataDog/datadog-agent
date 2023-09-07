// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics implements everything related to metrics of the pkg/compliance module.
package metrics

const (
	// MetricInputsHits is the metric name for counting inputs resolution.
	MetricInputsHits = "datadog.security_agent.compliance.inputs.hits"

	// MetricInputsDuration is the metric name for evaluating inputs resolution duration.
	MetricInputsDuration = "datadog.security_agent.compliance.inputs.duration_ms"

	// MetricInputsSize is the metric name for counting the total evaluated input size going through the rego evaluation engine.
	MetricInputsSize = "datadog.security_agent.compliance.inputs.size"

	// MetricChecksStatuses is the metric name for checks statuses of our evaluated rules.
	MetricChecksStatuses = "datadog.security_agent.compliance.checks.statuses"
)
