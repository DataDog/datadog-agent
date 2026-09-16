// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package severityprovider

import severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"

// provider is set once at startup (see SetSeverityProvider) by the fx wiring that resolves
// the Component, and read continuously by the log decoder's adaptive sampler. The decoder
// lives under pkg/logs/internal and cannot depend on the fx-wired command layer directly, so
// this package-level indirection — living in this def package rather than the sampler's own
// tree — is what lets both sides reach it.
var provider func() (severityeventsdef.SeverityLevel, bool)

// SetSeverityProvider registers the function used to read the current anomaly-detection
// severity level. Called once at startup, typically with a Component's own Current method.
func SetSeverityProvider(fn func() (severityeventsdef.SeverityLevel, bool)) {
	provider = fn
}

// Current returns the current anomaly-detection severity level via the registered provider,
// or (SeverityLow, false) if none was registered yet.
func Current() (severityeventsdef.SeverityLevel, bool) {
	if provider == nil {
		return severityeventsdef.SeverityLow, false
	}
	return provider()
}
