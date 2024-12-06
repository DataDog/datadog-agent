// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import "github.com/DataDog/datadog-agent/pkg/process/procutil"

const (
	maxCommandLine = 5000
)

// countAndAddElements is a helper for truncateCmdline used to be able to
// pre-calculate the size of the output slice to improve performance.
func countAndAddElements(cmdline []string, inElements int) (int, []string) {
	var out []string

	if inElements != 0 {
		out = make([]string, 0, inElements)
	}

	elements := 0
	total := 0
	for _, arg := range cmdline {
		if total >= maxCommandLine {
			break
		}

		this := len(arg)
		if this == 0 {
			// To avoid ending up with a large array with empty strings
			continue
		}

		if total+this > maxCommandLine {
			this = maxCommandLine - total
		}

		if inElements != 0 {
			out = append(out, arg[:this])
		}

		elements++
		total += this
	}

	return elements, out
}

// truncateCmdline truncates the command line length to maxCommandLine.
func truncateCmdline(cmdline []string) []string {
	elements, _ := countAndAddElements(cmdline, 0)
	_, out := countAndAddElements(cmdline, elements)
	return out
}

// sanitizeCmdLine scubs the command line of sensitive data and truncates it
// to a fixed size to limit memory usage.
func sanitizeCmdLine(scrubber *procutil.DataScrubber, cmdline []string) []string {
	cmdline, _ = scrubber.ScrubCommand(cmdline)
	return truncateCmdline(cmdline)
}
