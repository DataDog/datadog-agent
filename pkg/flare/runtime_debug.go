// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

const maxDepsInFlare = 50

func provideRuntimeDebugInfo(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc("runtime_debug_info.log", getRuntimeDebugInfo)
}

func getRuntimeDebugInfo() ([]byte, error) {
	var buf bytes.Buffer

	writeBuildInfo(&buf)
	writeGCSettings(&buf)
	writeGCStats(&buf)

	return buf.Bytes(), nil
}

func writeBuildInfo(buf *bytes.Buffer) {
	fmt.Fprintf(buf, "=== Build Info ===\n")
	info, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Fprintf(buf, "Build info not available\n\n")
		return
	}

	fmt.Fprintf(buf, "Go Version: %s\n", info.GoVersion)
	fmt.Fprintf(buf, "Path: %s\n", info.Path)
	if info.Main.Path != "" {
		fmt.Fprintf(buf, "Main Module: %s@%s\n", info.Main.Path, info.Main.Version)
	}

	if len(info.Settings) > 0 {
		fmt.Fprintf(buf, "\nBuild Settings:\n")
		for _, s := range info.Settings {
			fmt.Fprintf(buf, "  %s = %s\n", s.Key, s.Value)
		}
	}

	if len(info.Deps) > 0 {
		fmt.Fprintf(buf, "\nDependencies (%d total):\n", len(info.Deps))
		limit := len(info.Deps)
		if limit > maxDepsInFlare {
			limit = maxDepsInFlare
		}
		for _, dep := range info.Deps[:limit] {
			if dep.Replace != nil {
				fmt.Fprintf(buf, "  %s@%s => %s@%s\n", dep.Path, dep.Version, dep.Replace.Path, dep.Replace.Version)
			} else {
				fmt.Fprintf(buf, "  %s@%s\n", dep.Path, dep.Version)
			}
		}
		if len(info.Deps) > maxDepsInFlare {
			fmt.Fprintf(buf, "  ... and %d more\n", len(info.Deps)-maxDepsInFlare)
		}
	}
	fmt.Fprintf(buf, "\n")
}

func writeGCSettings(buf *bytes.Buffer) {
	fmt.Fprintf(buf, "=== GC Settings ===\n")

	// Read GOGC and GOMEMLIMIT from environment variables rather than using
	// debug.SetGCPercent/debug.SetMemoryLimit, which mutate global runtime
	// state and could momentarily disable GC or remove memory limits.
	gogc := os.Getenv("GOGC")
	if gogc == "" {
		gogc = "not set (Go default: 100)"
	}
	fmt.Fprintf(buf, "GOGC: %s\n", gogc)

	memlimit := os.Getenv("GOMEMLIMIT")
	if memlimit == "" {
		memlimit = "not set (unlimited)"
	}
	fmt.Fprintf(buf, "GOMEMLIMIT: %s\n", memlimit)
	fmt.Fprintf(buf, "\n")
}

func writeGCStats(buf *bytes.Buffer) {
	fmt.Fprintf(buf, "=== GC Stats ===\n")
	var stats debug.GCStats
	debug.ReadGCStats(&stats)

	fmt.Fprintf(buf, "Last GC: %s\n", stats.LastGC)
	fmt.Fprintf(buf, "Num GC: %d\n", stats.NumGC)
	fmt.Fprintf(buf, "Pause Total: %s\n", stats.PauseTotal)

	if len(stats.Pause) > 0 {
		fmt.Fprintf(buf, "Recent Pauses (most recent first):\n")
		limit := len(stats.Pause)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(buf, "  %s\n", stats.Pause[i])
		}
		if len(stats.Pause) > 10 {
			fmt.Fprintf(buf, "  ... and %d more\n", len(stats.Pause)-10)
		}
	}

	if len(stats.PauseEnd) > 0 {
		fmt.Fprintf(buf, "Recent Pause End Times (most recent first):\n")
		limit := len(stats.PauseEnd)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(buf, "  %s\n", stats.PauseEnd[i])
		}
		if len(stats.PauseEnd) > 10 {
			fmt.Fprintf(buf, "  ... and %d more\n", len(stats.PauseEnd)-10)
		}
	}
	fmt.Fprintf(buf, "\n")
}
