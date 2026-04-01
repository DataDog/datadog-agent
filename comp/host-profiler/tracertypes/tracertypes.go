// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracertypes

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	PerlTracer Tracer = iota
	PHPTracer
	PythonTracer
	HotspotTracer
	RubyTracer
	V8Tracer
	DotnetTracer
	GoTracer
	Labels
	BEAMTracer

	tracerCount
)

var tracerNames = [...]string{
	PerlTracer:    "perl",
	PHPTracer:     "php",
	PythonTracer:  "python",
	HotspotTracer: "hotspot",
	RubyTracer:    "ruby",
	V8Tracer:      "v8",
	DotnetTracer:  "dotnet",
	GoTracer:      "go",
	Labels:        "labels",
	BEAMTracer:    "beam",
}

var tracerNameToType = map[string]Tracer{
	"perl":    PerlTracer,
	"php":     PHPTracer,
	"python":  PythonTracer,
	"hotspot": HotspotTracer,
	"ruby":    RubyTracer,
	"v8":      V8Tracer,
	"dotnet":  DotnetTracer,
	"go":      GoTracer,
	"labels":  Labels,
	"beam":    BEAMTracer,
}

// Tracer identifies a supported interpreter tracer.
type Tracer uint8

// IncludedTracers is the host-profiler tracer bitset.
type IncludedTracers uint16

func (t Tracer) String() string {
	if int(t) >= len(tracerNames) {
		return "<unknown>"
	}

	return tracerNames[t]
}

// AllTracers returns the full tracer bitset.
func AllTracers() IncludedTracers {
	var tracers IncludedTracers

	for tracer := Tracer(0); tracer < tracerCount; tracer++ {
		tracers.Enable(tracer)
	}

	return tracers
}

// IsMapEnabled reports whether an interpreter-specific BPF map should be loaded.
func IsMapEnabled(mapName string, includeTracers IncludedTracers) bool {
	switch mapName {
	case "perl_procs":
		return includeTracers.Has(PerlTracer)
	case "php_procs":
		return includeTracers.Has(PHPTracer)
	case "py_procs":
		return includeTracers.Has(PythonTracer)
	case "hotspot_procs":
		return includeTracers.Has(HotspotTracer)
	case "ruby_procs":
		return includeTracers.Has(RubyTracer)
	case "v8_procs":
		return includeTracers.Has(V8Tracer)
	case "dotnet_procs":
		return includeTracers.Has(DotnetTracer)
	case "beam_procs":
		return includeTracers.Has(BEAMTracer)
	case "go_labels_procs", "apm_int_procs":
		// These maps are referenced from unwind_stop and must always exist.
		return true
	default:
		return true
	}
}

// Parse parses and validates a comma-separated tracer list.
func Parse(raw string) (IncludedTracers, error) {
	var tracers IncludedTracers

	for name := range strings.SplitSeq(raw, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || name == "native" {
			continue
		}

		if name == "all" {
			tracers = AllTracers()
			continue
		}

		tracer, ok := tracerNameToType[name]
		if !ok {
			return 0, fmt.Errorf("unknown tracer: %s", name)
		}

		tracers.Enable(tracer)
	}

	if runtime.GOARCH == "arm64" {
		tracers.Disable(DotnetTracer)
	}

	return tracers, nil
}

// Has reports whether tracer is enabled.
func (t IncludedTracers) Has(tracer Tracer) bool {
	return t&(1<<tracer) != 0
}

// String renders the enabled tracers in enum order.
func (t IncludedTracers) String() string {
	names := make([]string, 0, tracerCount)
	for tracer := Tracer(0); tracer < tracerCount; tracer++ {
		if t.Has(tracer) {
			names = append(names, tracer.String())
		}
	}

	return strings.Join(names, ",")
}

// Enable turns on a tracer bit.
func (t *IncludedTracers) Enable(tracer Tracer) {
	*t |= 1 << tracer
}

// Disable clears a tracer bit.
func (t *IncludedTracers) Disable(tracer Tracer) {
	*t &= ^(1 << tracer)
}
