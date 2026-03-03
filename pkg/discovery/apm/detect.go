// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package apm provides functionality to detect the type of APM instrumentation a service is using.
package apm

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Instrumentation represents the state of APM instrumentation for a service.
type Instrumentation string

const (
	// None means the service is not instrumented with APM.
	None Instrumentation = "none"
	// Provided means the service has been manually instrumented.
	Provided Instrumentation = "provided"
)

type detector func(ctx usm.DetectionContext) Instrumentation

var (
	detectorMap = map[language.Language]detector{
		language.DotNet: dotNetDetector,
		language.Java:   javaDetector,
		language.Python: pythonDetector,
	}
)

// Detect attempts to detect the type of APM instrumentation for the given service.
func Detect(lang language.Language, ctx usm.DetectionContext, tracerMetadata *tracermetadata.TracerMetadata) Instrumentation {
	// if the process has valid tracer's metadata, then the
	// instrumentation is provided
	if tracerMetadata != nil {
		return Provided
	}

	// different detection for provided instrumentation for each
	if detect, ok := detectorMap[lang]; ok {
		return detect(ctx)
	}

	return None
}

func pythonDetectorFromMapsReader(reader io.Reader) Instrumentation {
	scanner := bufio.NewScanner(bufio.NewReader(reader))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "/ddtrace/") {
			return Provided
		}
	}

	return None
}

// pythonDetector detects the use of the ddtrace package in the process. Since
// the ddtrace package uses native libraries, the paths of these libraries will
// show up in /proc/$PID/maps.
//
// It looks for the "/ddtrace/" part of the path. It doesn not look for the
// "/site-packages/" part since some environments (such as pyinstaller) may not
// use that exact name.
//
// For example:
// 7aef453fc000-7aef453ff000 rw-p 0004c000 fc:06 7895473  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/_encoding.cpython-310-x86_64-linux-gnu.so
// 7aef45400000-7aef45459000 r--p 00000000 fc:06 7895588  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/datadog/profiling/libdd_wrapper.so
func pythonDetector(ctx usm.DetectionContext) Instrumentation {
	mapsPath := kernel.HostProc(strconv.Itoa(ctx.Pid), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return None
	}
	defer mapsFile.Close()

	return pythonDetectorFromMapsReader(mapsFile)
}

var javaAgentRegex = regexp.MustCompile(`-javaagent:.*(?:datadog|dd-java-agent|dd-trace-agent)\S*\.jar`)

func javaDetector(ctx usm.DetectionContext) Instrumentation {
	ignoreArgs := map[string]bool{
		"-version":     true,
		"-Xshare:dump": true,
		"/usr/share/ca-certificates-java/ca-certificates-java.jar": true,
	}

	// Check simple args on builtIn list.
	for _, v := range ctx.Args {
		if ignoreArgs[v] {
			return None
		}
		// don't instrument if javaagent is already there on the command line
		if javaAgentRegex.MatchString(v) {
			return Provided
		}
	}
	// also don't instrument if the javaagent is there in the environment variable JAVA_TOOL_OPTIONS and friends
	toolOptionEnvs := []string{
		// These are the environment variables that are used to pass options to the JVM
		"JAVA_TOOL_OPTIONS",
		"_JAVA_OPTIONS",
		"JDK_JAVA_OPTIONS",
		// I'm pretty sure these won't be necessary, as they should be parsed before the JVM sees them
		// but there's no harm in including them
		"JAVA_OPTIONS",
		"CATALINA_OPTS",
		"JDPA_OPTS",
	}
	for _, name := range toolOptionEnvs {
		if val, ok := ctx.Envs.Get(name); ok {
			if javaAgentRegex.MatchString(val) {
				return Provided
			}
		}
	}
	return None
}

func dotNetDetectorFromMapsReader(reader io.Reader) Instrumentation {
	scanner := bufio.NewScanner(bufio.NewReader(reader))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(line, "Datadog.Trace.dll") {
			return Provided
		}
	}

	return None
}

// dotNetDetector detects instrumentation in .NET applications.
//
// The primary check is for the environment variables which enables .NET
// profiling. This is required for auto-instrumentation, and besides that custom
// instrumentation using version 3.0.0 or later of Datadog.Trace requires
// auto-instrumentation. It is also set if some third-party
// profiling/instrumentation is active.
//
// The secondary check is to detect cases where an older version of
// Datadog.Trace is used for manual instrumentation without enabling
// auto-instrumentation. For this, we check for the presence of the DLL in the
// maps file. Note that this does not work for single-file deployments.
//
// 785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267 /home/foo/.../publish/Datadog.Trace.dll
func dotNetDetector(ctx usm.DetectionContext) Instrumentation {
	if val, ok := ctx.Envs.Get("CORECLR_ENABLE_PROFILING"); ok && val == "1" {
		return Provided
	}

	mapsPath := kernel.HostProc(strconv.Itoa(ctx.Pid), "maps")
	mapsFile, err := os.Open(mapsPath)
	if err != nil {
		return None
	}
	defer mapsFile.Close()

	return dotNetDetectorFromMapsReader(mapsFile)
}
