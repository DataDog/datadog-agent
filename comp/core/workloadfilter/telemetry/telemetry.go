// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package telemetry defines the telemetry for the workloadfilter component.
package telemetry

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	subsystem = "workloadfilter"
)

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

// Store contains all the telemetry for the workloadfilter component.
type Store struct {
	// Evaluations tracks the number of evaluations made by the workloadfilter.
	Evaluations telemetry.Counter
	// EvaluationErrors tracks the number of errors that occurred during evaluations.
	EvaluationErrors telemetry.Counter
	// RemoteEvaluations tracks the number of remote evaluations made by the workloadfilter.
	RemoteEvaluations telemetry.Counter
	// RemoteEvaluationErrors tracks the number of errors that occurred during remote evaluations.
	RemoteEvaluationErrors telemetry.Counter
	// CacheHits tracks the number of cache hits in the workloadfilter.
	CacheHits telemetry.Counter
	// CacheMisses tracks the number of cache misses in the workloadfilter.
	CacheMisses telemetry.Counter
}

// NewStore creates a new telemetry store for the workloadfilter component.
func NewStore(telemetryComp telemetry.Component) *Store {
	return &Store{
		Evaluations: telemetryComp.NewCounterWithOpts(
			subsystem,
			"evaluations",
			[]string{"workload_type", "program_name", "result"},
			"Number of evaluations made by the workloadfilter",
			commonOpts,
		),
		EvaluationErrors: telemetryComp.NewCounterWithOpts(
			subsystem,
			"evaluation_errors",
			[]string{"workload_type", "program_name", "error"},
			"Number of errors that occurred during evaluations",
			commonOpts,
		),
		// RemoteEvaluations tracks the number of remote evaluations made by the workloadfilter.
		// Remote
		RemoteEvaluations: telemetryComp.NewCounterWithOpts(
			subsystem,
			"remote_evaluations",
			[]string{"workload_type", "program_name", "result"},
			"Number of remote evaluations made by the workloadfilter",
			commonOpts,
		),
		// RemoteEvaluationErrors tracks the number of errors that occurred during remote evaluations.
		// Remote
		RemoteEvaluationErrors: telemetryComp.NewCounterWithOpts(
			subsystem,
			"remote_evaluation_errors",
			[]string{"workload_type", "program_name", "error"},
			"Number of errors that occurred during remote evaluations",
			commonOpts,
		),
		// CacheHits tracks the number of cache hits in the workloadfilter.
		// Remote
		CacheHits: telemetryComp.NewCounterWithOpts(
			subsystem,
			"cache_hits",
			[]string{"workload_type", "program_name", "result"},
			"Number of cache hits in the workloadfilter",
			commonOpts,
		),
		// CacheMisses tracks the number of cache misses in the workloadfilter.
		// Remote
		CacheMisses: telemetryComp.NewCounterWithOpts(
			subsystem,
			"cache_misses",
			[]string{"workload_type", "program_name"},
			"Number of cache misses in the workloadfilter",
			commonOpts,
		),
	}
}
