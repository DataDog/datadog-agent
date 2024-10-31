// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package apm provides functionality to detect the type of APM instrumentation a service is using.
package apm

import (
	"bufio"
	"debug/elf"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Instrumentation represents the state of APM instrumentation for a service.
type Instrumentation string

const (
	// None means the service is not instrumented with APM.
	None Instrumentation = "none"
	// Provided means the service has been manually instrumented.
	Provided Instrumentation = "provided"
	// Injected means the service is using automatic APM injection.
	Injected Instrumentation = "injected"
)

type detector func(ctx usm.DetectionContext) Instrumentation

var (
	detectorMap = map[language.Language]detector{
		language.DotNet: dotNetDetector,
		language.Java:   javaDetector,
		language.Node:   nodeDetector,
		language.Python: pythonDetector,
		language.Go:     goDetector,
	}

	nodeAPMCheckRegex = regexp.MustCompile(`"dd-trace"`)
)

// Detect attempts to detect the type of APM instrumentation for the given service.
func Detect(lang language.Language, ctx usm.DetectionContext) Instrumentation {
	// first check to see if the DD_INJECTION_ENABLED is set to tracer
	if isInjected(ctx.Envs) {
		return Injected
	}

	// different detection for provided instrumentation for each
	if detect, ok := detectorMap[lang]; ok {
		return detect(ctx)
	}

	return None
}

func isInjected(envs envs.Variables) bool {
	if val, ok := envs.Get("DD_INJECTION_ENABLED"); ok {
		parts := strings.Split(val, ",")
		for _, v := range parts {
			if v == "tracer" {
				return true
			}
		}
	}
	return false
}

const (
	// ddTraceGoPrefix is the prefix of the dd-trace-go symbols. The symbols we
	// are looking for are for example
	// "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.init". We use a prefix
	// without the version number instead of a specific symbol name in an
	// attempt to make it future-proof.
	ddTraceGoPrefix = "gopkg.in/DataDog/dd-trace-go"
	// ddTraceGoMaxLength is the maximum length of the dd-trace-go symbols which
	// we look for. The max length is an optimization in bininspect to avoid
	// reading unnecesssary symbols.  As of writing, most non-internal symbols
	// in dd-trace-go are under 100 chars. The tracer.init example above at 51
	// chars is one of the shortest.
	ddTraceGoMaxLength = 100
)

// goDetector detects APM instrumentation for Go binaries by checking for
// the presence of the dd-trace-go symbols in the ELF. This only works for
// unstripped binaries.
func goDetector(ctx usm.DetectionContext) Instrumentation {
	exePath := kernel.HostProc(strconv.Itoa(ctx.Pid), "exe")

	elfFile, err := elf.Open(exePath)
	if err != nil {
		log.Debugf("Unable to open exe %s: %v", exePath, err)
		return None
	}
	defer elfFile.Close()

	if _, err = bininspect.GetAnySymbolWithPrefix(elfFile, ddTraceGoPrefix, ddTraceGoMaxLength); err == nil {
		return Provided
	}

	// We failed to find symbols in the regular symbols section, now we can try the pclntab
	if _, err = bininspect.GetAnySymbolWithPrefixPCLNTAB(elfFile, ddTraceGoPrefix, ddTraceGoMaxLength); err == nil {
		return Provided
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

// isNodeInstrumented parses the provided `os.File` trying to find an
// entry for APM NodeJS instrumentation. Returns true if finding such
// an entry, false otherwise.
func isNodeInstrumented(f fs.File) bool {
	reader, err := usm.SizeVerifiedReader(f)
	if err != nil {
		return false
	}

	bufferedReader := bufio.NewReader(reader)

	return nodeAPMCheckRegex.MatchReader(bufferedReader)
}

// nodeDetector checks if a service has APM NodeJS instrumentation.
//
// To check for APM instrumentation, we try to find a package.json in
// the parent directories of the service. If found, we then check for a
// `dd-trace` entry to be present.
func nodeDetector(ctx usm.DetectionContext) Instrumentation {
	pkgJSONPath, ok := ctx.ContextMap[usm.NodePackageJSONPath]
	if !ok {
		log.Debugf("could not get package.json path from context map")
		return None
	}

	fs, ok := ctx.ContextMap[usm.ServiceSubFS]
	if !ok {
		log.Debugf("could not get SubFS for package.json")
		return None
	}

	pkgJSONFile, err := fs.(usm.SubDirFS).Open(pkgJSONPath.(string))
	if err != nil {
		log.Debugf("could not open package.json: %s", err)
		return None
	}
	defer pkgJSONFile.Close()

	if isNodeInstrumented(pkgJSONFile) {
		return Provided
	}

	return None
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
