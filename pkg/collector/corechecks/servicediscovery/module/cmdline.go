// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
)

const (
	maxCommandLine = 5000
	// Allow the full class path if it's not a large contributor to the full
	// command line length. The large command line could be due to other things
	// like arguments to the jar/class itself (this is the case for example for
	// Spark jobs).
	maxClassPath = maxCommandLine * 0.05
	// Always preserve a few entries in the classpath, to be able to debug
	// service name generation issues.
	numPreserveClassPathEntries = 3
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

func trimClassPath(classPath string) string {
	if len(classPath) <= maxClassPath {
		return classPath
	}

	parts := strings.Split(classPath, ":")
	if len(parts) <= numPreserveClassPathEntries {
		return classPath
	}

	// The ... is added to indicate that we have trimmed the classpath.
	parts = append(parts[:numPreserveClassPathEntries], "...")
	return strings.Join(parts, ":")
}

// trimJavaClassPathFromCommandLine reduces the size of class paths from Java
// command lines since they can sometimes be very large (thousands of
// characters) and lead to more important arguments (such as the name of the
// class being executed) being dropped due to command line size constraints.
//
// Note this could end up trimming long arguments called -cp to the class itself
// (java -cp foo org.MyClass -cp blah); we ignore that case for now since it
// seems unlikely.
func trimJavaClassPathFromCommandLine(cmdline []string) []string {
	var out []string

	prev := ""
	seenJar := false
	for _, arg := range cmdline {
		outArg := arg

		if !seenJar {
			if prev == "-cp" || prev == "-classpath" || prev == "--class-path" {
				outArg = trimClassPath(arg)
			} else if strings.HasPrefix(arg, "--class-path=") {
				cparg := arg[len("--class-path="):]
				outArg = "--class-path=" + trimClassPath(cparg)
			}
		}

		if arg == "-jar" {
			// Things after this are aguments to the jar itself.
			seenJar = true
		}

		out = append(out, outArg)
		prev = arg
	}

	return out
}

// truncateCmdline truncates the command line length to maxCommandLine.
func truncateCmdline(lang language.Language, cmdline []string) []string {
	if lang == language.Java {
		length := 0
		for _, arg := range cmdline {
			length += len(arg)
		}
		if length > maxCommandLine {
			cmdline = trimJavaClassPathFromCommandLine(cmdline)
		}
	}

	elements, _ := countAndAddElements(cmdline, 0)
	_, out := countAndAddElements(cmdline, elements)
	return out
}
