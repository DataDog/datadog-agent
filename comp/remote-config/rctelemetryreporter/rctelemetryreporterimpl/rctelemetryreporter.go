// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rctelemetryreporterimpl provides a DdRcTelemetryReporter that sends RC-specific metrics to the DD backend.
package rctelemetryreporterimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Telemetry telemetry.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDdRcTelemetryReporter),
	)
}

// DdRcTelemetryReporter is a datadog-agent telemetry counter for RC cache bypass metrics. It implements the RcTelemetryReporter interface.
type DdRcTelemetryReporter struct {
	BypassRateLimitCounter telemetry.Counter
	BypassTimeoutCounter   telemetry.Counter

	ConfigSubscriptionsActive              telemetry.Gauge
	ConfigSubscriptionClientsTracked       telemetry.Gauge
	ConfigSubscriptionsConnectedCounter    telemetry.Counter
	ConfigSubscriptionsDisconnectedCounter telemetry.Counter
}

// IncRateLimit increments the DdRcTelemetryReporter BypassRateLimitCounter counter.
func (r *DdRcTelemetryReporter) IncRateLimit() {
	if r.BypassRateLimitCounter != nil {
		r.BypassRateLimitCounter.Inc()
	}
}

// IncTimeout increments the DdRcTelemetryReporter BypassTimeoutCounter counter.
func (r *DdRcTelemetryReporter) IncTimeout() {
	if r.BypassTimeoutCounter != nil {
		r.BypassTimeoutCounter.Inc()
	}
}

// IncConfigSubscriptionsConnectedCounter increments the DdRcTelemetryReporter
// ConfigSubscriptionsConnectedCounter counter.
func (r *DdRcTelemetryReporter) IncConfigSubscriptionsConnectedCounter() {
	if r.ConfigSubscriptionsConnectedCounter != nil {
		r.ConfigSubscriptionsConnectedCounter.Inc()
	}
}

// IncConfigSubscriptionsDisconnectedCounter increments the
// DdRcTelemetryReporter ConfigSubscriptionsDisconnectedCounter counter.
func (r *DdRcTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {
	if r.ConfigSubscriptionsDisconnectedCounter != nil {
		r.ConfigSubscriptionsDisconnectedCounter.Inc()
	}
}

// SetConfigSubscriptionsActive sets the DdRcTelemetryReporter
// ConfigSubscriptionsActive gauge to the given value.
func (r *DdRcTelemetryReporter) SetConfigSubscriptionsActive(value int) {
	if r.ConfigSubscriptionsActive != nil {
		r.ConfigSubscriptionsActive.Set(float64(value))
	}
}

// SetConfigSubscriptionClientsTracked sets the DdRcTelemetryReporter
// ConfigSubscriptionClientsTracked gauge to the given value.
func (r *DdRcTelemetryReporter) SetConfigSubscriptionClientsTracked(value int) {
	if r.ConfigSubscriptionClientsTracked != nil {
		r.ConfigSubscriptionClientsTracked.Set(float64(value))
	}
}

// newDdRcTelemetryReporter creates a new Remote Config telemetry reporter for sending RC metrics to Datadog
func newDdRcTelemetryReporter(deps dependencies) rctelemetryreporter.Component {
	commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &DdRcTelemetryReporter{
		BypassRateLimitCounter: deps.Telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_ratelimiter_skip",
			[]string{},
			"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
			commonOpts,
		),
		BypassTimeoutCounter: deps.Telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_timeout",
			[]string{},
			"Number of Remote Configuration cache bypass requests that timeout.",
			commonOpts,
		),
		ConfigSubscriptionsActive: deps.Telemetry.NewGaugeWithOpts(
			"remoteconfig",
			"config_subscriptions_active",
			[]string{},
			"Number of Remote Configuration subscriptions active.",
			commonOpts,
		),
		ConfigSubscriptionClientsTracked: deps.Telemetry.NewGaugeWithOpts(
			"remoteconfig",
			"config_subscription_clients_tracked",
			[]string{},
			"Number of Remote Configuration clients tracked by active subscriptions.",
			commonOpts,
		),
		ConfigSubscriptionsConnectedCounter: deps.Telemetry.NewCounterWithOpts(
			"remoteconfig",
			"config_subscriptions_connected_counter",
			[]string{},
			"Number of Remote Configuration subscriptions connected.",
			commonOpts,
		),
		ConfigSubscriptionsDisconnectedCounter: deps.Telemetry.NewCounterWithOpts(
			"remoteconfig",
			"config_subscriptions_disconnected_counter",
			[]string{},
			"Number of Remote Configuration subscriptions disconnected.",
			commonOpts,
		),
	}
}
