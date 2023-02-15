// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

const (
	metricCompliancePrefix = "datadog.security_agent.compliance."

	// MetricInputsHits is the metric name for counting inputs resolution.
	MetricInputsHits = metricCompliancePrefix + "inputs.hits"

	// MetricInputsDuration is the metric name for evaluating inputs resolution duration.
	MetricInputsDuration = metricCompliancePrefix + "inputs.duration_ms"

	// MetricChecksStatuses is the metric name for checks statuses of our evaluated rules.
	MetricChecksStatuses = metricCompliancePrefix + "checks.statuses"
)
