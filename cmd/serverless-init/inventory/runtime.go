// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pythonExecutable = regexp.MustCompile(`^python([0-9]+(\.[0-9]+)*)?$`)

// resolveRuntime reports the application runtime, never the Agent or container
// runtime. Sidecars must pass no wrapped command: their argv is not the workload.
func resolveRuntime(metadata []string, wrappedCommand []string) string {
	candidates := append([]string{os.Getenv("DD_SERVERLESS_INVENTORY_RUNTIME")}, metadata...)
	candidates = append(candidates, commandRuntime(wrappedCommand))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		switch strings.ToLower(candidate) {
		case "", "unknown", "container", "null":
			continue
		default:
			return candidate
		}
	}
	return ""
}

func commandRuntime(wrappedCommand []string) string {
	if len(wrappedCommand) == 0 {
		return ""
	}

	// Only recognize an obvious executable. Do not parse shell expressions,
	// unwrap launchers, inspect files/processes, or infer runtime versions.
	executable := filepath.Base(wrappedCommand[0])
	switch executable {
	case "node", "nodejs":
		return "Node.js"
	case "java":
		return "Java"
	case "dotnet":
		return ".NET"
	case "php":
		return "PHP"
	case "ruby":
		return "Ruby"
	}
	if pythonExecutable.MatchString(executable) {
		return "Python"
	}
	return ""
}
