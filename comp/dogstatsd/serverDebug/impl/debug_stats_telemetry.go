// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverdebugimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/seriesstats"
)

var defaultDebugStatsViewTelemetry = newDebugStatsViewTelemetry(telemetryimpl.GetCompatComponent())

type telemetryDebugStatsView struct {
	storedContexts   telemetry.Gauge
	budgetEvictions  telemetry.Counter
	ttlPrunes        telemetry.Counter
	snapshots        telemetry.Counter
	snapshotContexts telemetry.Gauge
}

func newDebugStatsViewTelemetry(comp telemetry.Component) seriesstats.Telemetry {
	return &telemetryDebugStatsView{
		storedContexts: comp.NewGauge(
			"dogstatsd",
			"debug_stats_contexts",
			nil,
			"Current number of DogStatsD debug stats contexts retained in the bounded view."),
		budgetEvictions: comp.NewCounter(
			"dogstatsd",
			"debug_stats_evictions_total",
			nil,
			"Number of DogStatsD debug stats contexts evicted because the bounded view was full."),
		ttlPrunes: comp.NewCounter(
			"dogstatsd",
			"debug_stats_ttl_prunes_total",
			nil,
			"Number of DogStatsD debug stats contexts pruned after exceeding the retention TTL."),
		snapshots: comp.NewCounter(
			"dogstatsd",
			"debug_stats_snapshots_total",
			nil,
			"Number of DogStatsD debug stats snapshots built for the dogstatsd-stats endpoint."),
		snapshotContexts: comp.NewGauge(
			"dogstatsd",
			"debug_stats_snapshot_contexts",
			nil,
			"Number of DogStatsD debug stats contexts included in the most recent snapshot."),
	}
}

func (t *telemetryDebugStatsView) SetStoredContexts(count int) {
	t.storedContexts.Set(float64(count))
}

func (t *telemetryDebugStatsView) IncBudgetEvictions() {
	t.budgetEvictions.Inc()
}

func (t *telemetryDebugStatsView) AddTTLPrunes(count int) {
	if count > 0 {
		t.ttlPrunes.Add(float64(count))
	}
}

func (t *telemetryDebugStatsView) IncSnapshots() {
	t.snapshots.Inc()
}

func (t *telemetryDebugStatsView) SetSnapshotContexts(count int) {
	t.snapshotContexts.Set(float64(count))
}
