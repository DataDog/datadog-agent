package rctelemetryreporter

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// team: remote-config

// DdRcTelemetryReporter is a datadog-agent telemetry counter for RC cache bypass metrics. It implements the RcTelemetryReporter interface.
type DdRcTelemetryReporter struct {
	BypassRateLimitCounter telemetry.Counter
	BypassTimeoutCounter   telemetry.Counter
}

// IncRateLimit increments the DdRcTelemetryReporter BypassRateLimitCounter counter.
func (r *DdRcTelemetryReporter) IncRateLimit() {
	r.BypassRateLimitCounter.Inc()
}

// IncTimeout increments the DdRcTelemetryReporter BypassTimeoutCounter counter.
func (r *DdRcTelemetryReporter) IncTimeout() {
	r.BypassTimeoutCounter.Inc()
}

// NewDdRcTelemetryReporter creates a new Remote Config telemetry reporter for sending RC metrics to Datadog
func NewDdRcTelemetryReporter() Component {
	commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &DdRcTelemetryReporter{
		BypassRateLimitCounter: telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_ratelimiter_skip",
			[]string{},
			"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
			commonOpts,
		),
		BypassTimeoutCounter: telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_timeout",
			[]string{},
			"Number of Remote Configuration cache bypass requests that timeout.",
			commonOpts,
		),
	}
}
