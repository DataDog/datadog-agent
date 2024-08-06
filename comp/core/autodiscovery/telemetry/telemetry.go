// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry defines the Autodiscovery telemetry metrics.
package telemetry

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	subsystem = "autodiscovery"

	// ResourceKubeService represents kubernetes service entities
	ResourceKubeService = "k8s_service"
	// ResourceKubeEndpoint represents kubernetes endpoint entities
	ResourceKubeEndpoint = "k8s_endpoint"
)

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

// Store holds all the telemetry metrics for Autodiscovery.
type Store struct {
	// ScheduledConfigs tracks how many configs are scheduled.
	ScheduledConfigs telemetry.Gauge
	// WatchedResources tracks how many resources are watched by AD listeners.
	WatchedResources telemetry.Gauge
	// Errors tracks the current number of AD configs with errors by AD providers.
	Errors telemetry.Gauge
	// PollDuration tracks the configs poll duration by AD providers.
	PollDuration telemetry.Histogram
}

// NewStore returns a new Store.
func NewStore(telemetryComp telemetry.Component) *Store {
	return &Store{
		ScheduledConfigs: telemetryComp.NewGaugeWithOpts(
			subsystem,
			"scheduled_configs",
			[]string{"provider", "type"},
			"Number of configs scheduled in Autodiscovery by provider and type.",
			commonOpts,
		),
		WatchedResources: telemetryComp.NewGaugeWithOpts(
			subsystem,
			"watched_resources",
			[]string{"listener", "kind"},
			"Number of resources watched in Autodiscovery by listener and kind.",
			commonOpts,
		),
		Errors: telemetryComp.NewGaugeWithOpts(
			subsystem,
			"errors",
			[]string{"provider"},
			"Current number of Autodiscovery configs with errors by provider.",
			commonOpts,
		),
		PollDuration: telemetryComp.NewHistogramWithOpts(
			subsystem,
			"poll_duration",
			[]string{"provider"},
			"Poll duration distribution by config provider (in seconds).",
			// The default prometheus buckets are adapted to measure response time of network services
			prometheus.DefBuckets,
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
	}
}
