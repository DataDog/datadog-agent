// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package severityprovider defines the severity provider component.
package severityprovider

import severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"

// team: agent-log-pipelines q-branch

// Component exposes the current anomaly-detection severity to log samplers.
type Component interface {
	// Current returns the current severity level, if available.
	Current() (severityeventsdef.SeverityLevel, bool)
}
