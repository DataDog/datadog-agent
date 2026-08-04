// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import "sync/atomic"

// Metrics counts the work done by a Scanner.
//
// It is written by Scan and may be read concurrently.
type Metrics struct {
	// candidatesEvaluated counts processes that made it past the age floor,
	// the already-instrumented check and the retry schedule, and therefore
	// cost us at least one look at their executable.
	candidatesEvaluated atomic.Uint64
	// executablesAnalyzed counts executables parsed to decide whether they are
	// Go binaries, i.e. misses in the executable cache.
	executablesAnalyzed atomic.Uint64
	// nonGoExecutables counts evaluations skipped because the executable is
	// not a Go binary.
	nonGoExecutables atomic.Uint64
	// executablesUnresolved counts evaluations that could not identify the
	// process' executable at all, which is what every kernel thread does.
	executablesUnresolved atomic.Uint64
	// nonGoTracers counts Go binaries whose tracer reports a language other
	// than Go, which we have no way to instrument.
	nonGoTracers atomic.Uint64
	// executablesChanged counts processes found running a different executable
	// than the one the last verdict about them was reached about, i.e. execs.
	// Their retry schedule starts over.
	executablesChanged atomic.Uint64
	// discovered counts processes reported as newly instrumentable.
	discovered atomic.Uint64
	// metadataNotPublished counts metadata reads that found no tracer memfd.
	// The tracer may still publish one later, so these resolve on their own.
	metadataNotPublished atomic.Uint64
	// metadataUnreadable counts metadata reads that failed for a reason other
	// than the file being absent, most notably a lack of permission. These do
	// not resolve on their own and need someone to look at them.
	metadataUnreadable atomic.Uint64
	// candidatesEvicted counts candidates dropped because the candidate set is
	// full. An evicted candidate loses its retry schedule, not its eligibility.
	candidatesEvicted atomic.Uint64
}

// asStats converts the Metrics to a map[string]any for use by the system-probe.
func (m *Metrics) asStats() map[string]any {
	return map[string]any{
		"candidates_evaluated":   m.candidatesEvaluated.Load(),
		"executables_analyzed":   m.executablesAnalyzed.Load(),
		"non_go_executables":     m.nonGoExecutables.Load(),
		"executables_unresolved": m.executablesUnresolved.Load(),
		"non_go_tracers":         m.nonGoTracers.Load(),
		"executables_changed":    m.executablesChanged.Load(),
		"discovered":             m.discovered.Load(),
		"metadata_not_published": m.metadataNotPublished.Load(),
		"metadata_unreadable":    m.metadataUnreadable.Load(),
		"candidates_evicted":     m.candidatesEvicted.Load(),
	}
}
