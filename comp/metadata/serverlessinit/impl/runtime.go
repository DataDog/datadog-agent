// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package serverlessinitimpl

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

// runtimeUnknown is the value reported when no candidate identifies the
// application runtime.
const runtimeUnknown = ""

var pythonExecutable = regexp.MustCompile(`^python([0-9]+(\.[0-9]+)*)?$`)

// resolveRuntime reports the application runtime, never the Agent or container
// runtime. Sidecars must pass no wrapped command: their argv is not the workload.
func resolveRuntime(metadata []string, wrappedCommand []string) string {
	candidates := append([]string{os.Getenv("DD_SERVERLESS_METADATA_RUNTIME")}, metadata...)
	candidates = append(candidates, commandRuntime(wrappedCommand))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		switch strings.ToLower(candidate) {
		case runtimeUnknown, "unknown", "container", "null":
			continue
		default:
			return candidate
		}
	}
	return runtimeUnknown
}

func commandRuntime(wrappedCommand []string) string {
	if len(wrappedCommand) == 0 {
		return runtimeUnknown
	}

	// Only recognize obvious executables and explicit launcher patterns. Do not
	// parse shell expressions, inspect files/processes, or infer runtime versions.
	switch filepath.Base(wrappedCommand[0]) {
	case "ddtrace-run":
		if len(wrappedCommand) > 1 && executableRuntime(wrappedCommand[1]) == serverlessdef.RuntimePython {
			return serverlessdef.RuntimePython
		}
	case "bundle":
		if len(wrappedCommand) > 2 && wrappedCommand[1] == "exec" && executableRuntime(wrappedCommand[2]) == serverlessdef.RuntimeRuby {
			return serverlessdef.RuntimeRuby
		}
	}
	return executableRuntime(wrappedCommand[0])
}

func executableRuntime(executable string) string {
	executable = filepath.Base(executable)
	switch executable {
	case "node", "nodejs", "npm":
		return serverlessdef.RuntimeNodeJS
	case "java":
		return serverlessdef.RuntimeJava
	case "dotnet":
		return serverlessdef.RuntimeDotNet
	case "php":
		return serverlessdef.RuntimePHP
	case "ruby", "rails", "rake", "puma", "sidekiq":
		return serverlessdef.RuntimeRuby
	case "gunicorn", "uvicorn", "flask", "celery":
		return serverlessdef.RuntimePython
	}
	if pythonExecutable.MatchString(executable) {
		return serverlessdef.RuntimePython
	}
	return runtimeUnknown
}
