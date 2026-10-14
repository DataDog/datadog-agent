// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package model contains the types exchanged between the jvm_memory
// system-probe module and the jvm_memory core check.
package model

import "math"

// UndefinedSize is the value reported by HotSpot for a memory pool size that
// is not defined (for instance the maximum size of a pool without limit).
// It is HotSpot's SIZE_MAX, forwarded as is.
const UndefinedSize uint64 = math.MaxUint64

// JVMStats contains the memory and GC statistics of a single HotSpot JVM,
// as returned by the `/jvm_memory/check` system-probe endpoint.
type JVMStats struct {
	// PID is the process id of the JVM, in the PID namespace of the procfs
	// used by system-probe (the host PID namespace in every real deployment).
	PID uint32 `json:"pid"`
	// ContainerID is the id of the container running the JVM, empty for a
	// JVM running directly on the host.
	ContainerID string `json:"container_id,omitempty"`
	// Pools contains the state of every memory pool of the JVM, as reported
	// at the end of the last garbage collection that touched it.
	Pools []PoolStats `json:"pools"`
	// GC contains the cumulative stop-the-world pause counters of the JVM.
	GC GCStats `json:"gc"`
}

// PoolStats contains the state of a JVM memory pool right after a garbage
// collection.
type PoolStats struct {
	// Name is the raw HotSpot name of the pool (e.g. "G1 Eden Space",
	// "CodeHeap 'profiled nmethods'").
	Name string `json:"name"`
	// Init is the initial size of the pool in bytes, UndefinedSize when undefined.
	Init uint64 `json:"init"`
	// Used is the number of bytes used in the pool.
	Used uint64 `json:"used"`
	// Committed is the number of bytes committed for the pool.
	Committed uint64 `json:"committed"`
	// Max is the maximum size of the pool in bytes, UndefinedSize when undefined.
	Max uint64 `json:"max"`
	// LastUpdate is the time of the last update of the pool, in nanoseconds
	// since the Unix epoch (wall clock).
	LastUpdate int64 `json:"last_update"`
}

// GCStats contains the cumulative stop-the-world pause counters of a JVM.
//
// The counters bracket pauses, not collections: concurrent collectors report
// several pauses per collection cycle.
type GCStats struct {
	// Pauses is the number of completed pauses.
	Pauses uint64 `json:"pauses"`
	// FullPauses is the number of completed pauses that were full collections.
	FullPauses uint64 `json:"full_pauses"`
	// PauseTimeNs is the cumulative duration of the completed pauses, in nanoseconds.
	PauseTimeNs uint64 `json:"pause_time_ns"`
	// LastPauseEnd is the end time of the last completed pause, in nanoseconds
	// since the Unix epoch (wall clock), 0 if no pause completed yet.
	LastPauseEnd int64 `json:"last_pause_end"`
}
