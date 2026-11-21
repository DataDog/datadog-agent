// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fxinstrumentationimpl

import (
	"os"
	"path/filepath"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/lifecycle"
)

const (
	subsystem = "runtime"

	// LifecycleRecordFilePrefix is the prefix of the file storing the lifecycle record of a process in
	// the run path.
	LifecycleRecordFilePrefix = "lifecycle-"
)

// durationBuckets covers fast starts (sub-second) to the slowest shutdowns we have seen (minutes).
var durationBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300, 600}

// LifecycleTelemetry publishes the lifecycle timings of the process as internal telemetry. These
// metrics are reported to Datadog by the agent telemetry component (see the "lifecycle" profile in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml).
type LifecycleTelemetry struct {
	startupDuration         telemetry.Histogram
	startupSlowComponent    telemetry.Histogram
	previousShutdown        telemetry.Histogram
	previousShutdownSlowest telemetry.Histogram
	previousExit            telemetry.Counter
}

// NewLifecycleTelemetry registers the lifecycle metrics.
func NewLifecycleTelemetry(tm telemetry.Component) *LifecycleTelemetry {
	return &LifecycleTelemetry{
		startupDuration: tm.NewHistogram(subsystem, "startup_duration_seconds",
			[]string{"phase"},
			"Duration of the startup phases of the process, in seconds",
			durationBuckets),
		startupSlowComponent: tm.NewHistogram(subsystem, "startup_slow_component_seconds",
			[]string{"stage", "component"},
			"Duration of the slowest components during the startup of the process, in seconds",
			durationBuckets),
		previousShutdown: tm.NewHistogram(subsystem, "previous_shutdown_seconds",
			[]string{"phase", "outcome"},
			"Duration of the shutdown phases of the previous run of the process, in seconds",
			durationBuckets),
		previousShutdownSlowest: tm.NewHistogram(subsystem, "previous_shutdown_slow_component_seconds",
			[]string{"stage", "component", "outcome"},
			"Duration of the slowest components during the shutdown of the previous run of the process, in seconds",
			durationBuckets),
		previousExit: tm.NewCounter(subsystem, "previous_exit",
			[]string{"outcome", "trigger", "phase", "component"},
			"How the previous run of the process ended. For an unclean or failed shutdown, phase and component are where it was stuck"),
	}
}

// ReportStartup records the startup timings of the process.
func (t *LifecycleTelemetry) ReportStartup(s lifecycle.Startup) {
	for phase, seconds := range s.Phases {
		t.startupDuration.Observe(seconds, phase)
	}
	for _, c := range s.SlowComponents {
		t.startupSlowComponent.Observe(c.Seconds, c.Stage, c.Name)
	}
}

// ReportPrevious records how the previous run of the process ended.
func (t *LifecycleTelemetry) ReportPrevious(rec lifecycle.Record) {
	outcome := string(rec.Outcome())

	trigger := rec.Trigger
	phase, component := "", ""
	if o := rec.Outcome(); o == lifecycle.OutcomeKilledDuringShutdown || o == lifecycle.OutcomeShutdownFailed {
		phase, component = rec.CurrentPhase, rec.CurrentComponent
	}
	t.previousExit.Inc(outcome, trigger, phase, component)

	if rec.State == lifecycle.StateRunning {
		// The process did not begin its shutdown: there are no shutdown timings.
		return
	}
	for p, seconds := range rec.Phases {
		if p == lifecycle.PhaseTotal {
			continue
		}
		t.previousShutdown.Observe(seconds, p, outcome)
	}
	if total, ok := rec.ShutdownSeconds(); ok {
		t.previousShutdown.Observe(total, lifecycle.PhaseTotal, outcome)
	}
	for _, c := range rec.SlowComponents {
		t.previousShutdownSlowest.Observe(c.Seconds, c.Stage, c.Name, outcome)
	}
}

// RecordFilePath returns the path of the lifecycle record of the process flavor in runPath.
func RecordFilePath(runPath, flavor string) string {
	return filepath.Join(runPath, LifecycleRecordFilePrefix+strings.ReplaceAll(flavor, "_", "-")+".json")
}

// SetupLifecycleTelemetry reports the previous run of the process recorded in runPath, enables
// persisting the record of the current run, and reports the startup once the process is ready.
func SetupLifecycleTelemetry(recorder *lifecycle.Recorder, tm telemetry.Component, logger log.Component, runPath, flavor string) {
	t := NewLifecycleTelemetry(tm)

	if runPath == "" {
		logger.Debug("Run path is not set, lifecycle records of previous runs are not reported")
	} else if fi, err := os.Stat(runPath); err != nil || !fi.IsDir() {
		logger.Debugf("Run path %q is not a directory, lifecycle records of previous runs are not reported", runPath)
	} else {
		path := RecordFilePath(runPath, flavor)
		rec, found, err := lifecycle.LoadPrevious(path)
		if err != nil {
			logger.Debugf("Failed to load the lifecycle record of the previous run: %v", err)
		}
		if found {
			t.ReportPrevious(rec)
		}

		reported := false
		recorder.Persist(path, func(err error) {
			// Called with the recorder lock held: only log, and only once.
			if !reported {
				reported = true
				logger.Debugf("Failed to write the lifecycle record %s: %v", path, err)
			}
		})
	}

	recorder.OnReady(t.ReportStartup)
}
