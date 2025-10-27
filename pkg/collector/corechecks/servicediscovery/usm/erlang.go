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
	if name != "" && name != "beam" {
		return NewServiceMetadata(name, CommandLine), true
	}
	return ServiceMetadata{}, false
}

func detectErlangAppName(cmdline []string) string {
	var progname string
	var home string

	// Parse command line looking for -progname and -home flags.
	// We support both separated (-progname erl) and concatenated (-prognameerl) forms.
	for i := 0; i < len(cmdline); i++ {
		arg := cmdline[i]

		// Check for -progname flag (with value in next arg)
		if arg == "-progname" && i+1 < len(cmdline) {
			progname = strings.TrimSpace(cmdline[i+1])
			i++
			continue
		}

		// Check for -progname with concatenated value (e.g., -prognameerl)
		if strings.HasPrefix(arg, "-progname") && len(arg) > 9 {
			progname = strings.TrimSpace(arg[9:])
			continue
		}

		// Check for -home flag (with value in next arg)
		if arg == "-home" && i+1 < len(cmdline) {
			home = strings.TrimSpace(cmdline[i+1])
			i++
			continue
		}

		// Check for -home with concatenated value (e.g., -home/var/lib/rabbitmq)
		if strings.HasPrefix(arg, "-home") && len(arg) > 5 {
			home = strings.TrimSpace(arg[5:])
			continue
		}
	}

	// Apply heuristics according to requirements.
	// Compare progname case-insensitively to handle edge cases.
	if progname != "" && !strings.EqualFold(progname, "erl") {
		return progname
	}

	// Only use home if progname is explicitly "erl"
	if strings.EqualFold(progname, "erl") && home != "" {
		// Extract the last component of the home path
		base := filepath.Base(home)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}

	// Fallback to "beam"
	return "beam"
}
