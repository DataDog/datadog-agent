// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"path/filepath"
	"strings"
)

type erlangDetector struct {
	ctx DetectionContext
}

func newErlangDetector(ctx DetectionContext) detector {
	return &erlangDetector{ctx: ctx}
}

func (e erlangDetector) detect(args []string) (ServiceMetadata, bool) {
	name := detectErlangAppName(args)
	if name != "" {
		return NewServiceMetadata(name, CommandLine), true
	}
	return ServiceMetadata{}, false
}

func detectErlangAppName(cmdline []string) string {
	var progname string
	var home string

	// Parse command line looking for -progname and -home flags.
	// Erlang uses space-separated flag values (e.g., -progname erl).
	for i := 0; i < len(cmdline); i++ {
		arg := cmdline[i]

		// Check for -progname flag (with value in next arg)
		if arg == "-progname" && i+1 < len(cmdline) {
			progname = strings.TrimSpace(cmdline[i+1])
			i++
			continue
		}

		// Check for -home flag (with value in next arg)
		if arg == "-home" && i+1 < len(cmdline) {
			home = strings.TrimSpace(cmdline[i+1])
			i++
			continue
		}
	}

	// Apply heuristics according to requirements.
	// Use exact comparison for progname
	if progname != "" && progname != "erl" {
		return progname
	}

	// Only use home if progname is explicitly "erl"
	if progname == "erl" && home != "" {
		// Extract the last component of the home path
		base := filepath.Base(home)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}

	// Return empty string to indicate we couldn't extract a name
	return ""
}
