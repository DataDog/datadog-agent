// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

var emittedEvents = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	CheckName,
	"emitted_events",
	[]string{"event_type", "object_kind"},
	"Number of events emitted by the check",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

var queueLength = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
	CheckName,
	"subscription_queue_length",
	[]string{"channel"},
	"Number of event bundles waiting in a workloadmeta subscription channel, observed when a bundle is received",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

var queueLatency = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
	CheckName,
	"subscription_queue_latency_seconds",
	[]string{"channel"},
	"Time an event bundle waited in a workloadmeta subscription channel before the check received it (in seconds)",
	[]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

var bundleSize = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
	CheckName,
	"subscription_bundle_size",
	[]string{"channel"},
	"Number of events in each bundle received from a workloadmeta subscription channel",
	[]float64{1, 5, 10, 50, 100, 250, 500, 1000, 5000},
	telemetry.Options{NoDoubleUnderscoreSep: true},
)
