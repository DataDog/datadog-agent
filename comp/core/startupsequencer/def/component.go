// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startupsequencer spreads expensive subsystem startup work across
// time so that the agent does not allocate (and immediately free) the working
// set of every subsystem at once during boot.
//
// Subsystems register their start work with Defer instead of running it
// directly in their fx OnStart hook. When staged startup is disabled the work
// runs inline, exactly as it would have before, so disabling the feature is a
// no-op. When enabled, the work is grouped into ordered stages that the
// sequencer runs from a background goroutine, reclaiming transient memory
// between stages so the peak resident set stays close to the steady state.
package startupsequencer

import "context"

// team: agent-runtimes

// Stage identifies one step of the staged startup sequence. Lower stages run
// first. Subsystems that must be processing data for the agent to be
// considered up belong in the earlier stages; purely background work belongs
// in the later ones.
type Stage int

const (
	// StageCritical is the load-bearing path that must be up for the agent to
	// accept and forward data (forwarder, aggregator, API/health endpoints).
	StageCritical Stage = iota
	// StageIngest covers the data-ingestion front ends (DogStatsD, logs).
	StageIngest
	// StageChecks covers the collector and check scheduling.
	StageChecks
	// StageBackground covers everything that can come up last without any
	// user-observable effect (metadata/inventory collection, OTel, process
	// agent, network device monitoring, ...).
	StageBackground

	// NumStages is the number of defined stages. It must remain the last
	// constant in this block.
	NumStages
)

// Component runs deferred subsystem start work in ordered, memory-paced stages.
type Component interface {
	// Defer schedules fn to run during the given stage. name is used for
	// logging. When staged startup is disabled, fn runs synchronously and its
	// error is returned; this makes a Defer call from an OnStart hook behave
	// identically to running the work in the hook directly.
	Defer(stage Stage, name string, fn func(context.Context) error) error

	// Begin starts running the deferred work. It must be called once, after all
	// Defer calls have been made, by the owner of the startup sequence. When
	// staged startup is disabled it is a no-op (the work already ran inline).
	// ctx cancels the in-progress sequence (e.g. on shutdown).
	Begin(ctx context.Context)
}
