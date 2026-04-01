// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/log"
)

type tracerType int

const (
	PerlTracer tracerType = iota
	PHPTracer
	PythonTracer
	HotspotTracer
	RubyTracer
	V8Tracer
	DotnetTracer
	GoTracer
	Labels
	BEAMTracer

	maxTracers
)

var tracerTypeToName = map[tracerType]string{
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

var tracerNameToType = func() map[string]tracerType {
	names := make(map[string]tracerType, len(tracerTypeToName))
	for tracer, name := range tracerTypeToName {
		names[name] = tracer
	}
	return names
}()

// IncludedTracers holds information about which tracers are enabled.
type IncludedTracers uint16

// IsMapEnabled checks if the given map is enabled and should be loaded.
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
		return true
	default:
		return true
	}
}

func (t tracerType) String() string {
	if name, found := tracerTypeToName[t]; found {
		return name
	}
	return "<unknown>"
}

// String returns a comma-separated list of enabled tracers.
func (t *IncludedTracers) String() string {
	names := make([]string, 0, maxTracers)
	for tracer := tracerType(0); tracer < maxTracers; tracer++ {
		if t.Has(tracer) {
			names = append(names, tracer.String())
		}
	}
	return strings.Join(names, ",")
}

// Has returns true if the given tracer is enabled.
func (t *IncludedTracers) Has(tracer tracerType) bool {
	return *t&(1<<tracer) != 0
}

// Enable enables the given tracer.
func (t *IncludedTracers) Enable(tracer tracerType) {
	*t |= 1 << tracer
}

// Disable disables the given tracer.
func (t *IncludedTracers) Disable(tracer tracerType) {
	*t &= ^(1 << tracer)
}

// AllTracers returns an element with all tracers enabled.
func AllTracers() IncludedTracers {
	var result IncludedTracers
	for tracer := tracerType(0); tracer < maxTracers; tracer++ {
		result.Enable(tracer)
	}
	disableUnsupportedTracers(&result)
	return result
}

// Parse parses a string that specifies one or more eBPF tracers to enable.
func Parse(tracers string) (IncludedTracers, error) {
	var result IncludedTracers

	for name := range strings.SplitSeq(tracers, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || name == "native" {
			continue
		}

		if name == "all" {
			result = AllTracers()
			continue
		}

		tracer, ok := tracerNameToType[name]
		if !ok {
			return result, fmt.Errorf("unknown tracer: %s", name)
		}

		result.Enable(tracer)
	}

	if runtime.GOARCH == "arm64" {
		if result.Has(DotnetTracer) {
			result.Disable(DotnetTracer)
			log.Warn("The dotnet tracer is currently not supported on ARM64")
		}
	}
	disableUnsupportedTracers(&result)

	if tracersEnabled := result.String(); tracersEnabled != "" {
		log.Debugf("Tracer string: %v", tracers)
		log.Infof("Interpreter tracers: %v", tracersEnabled)
	}

	return result, nil
}

func disableUnsupportedTracers(tracers *IncludedTracers) {
	if tracers.Has(GoTracer) {
		tracers.Disable(GoTracer)
		log.Warn("The go tracer is not supported in host-profiler; Go is symbolized remotely")
	}
}
