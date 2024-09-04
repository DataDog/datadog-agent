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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language/reader"
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

type detector func(pid int, args []string, envs map[string]string, contextMap usm.DetectorContextMap) Instrumentation

var (
	detectorMap = map[language.Language]detector{
		language.DotNet: dotNetDetector,
		language.Java:   javaDetector,
		language.Node:   nodeDetector,
		language.Python: pythonDetector,
		language.Go:     goDetector,
	}
	// For now, only allow a subset of the above detectors to actually run.
	allowedLangs = map[language.Language]struct{}{
		language.Java:   {},
		language.Node:   {},
		language.Python: {},
		language.Go:     {},
	}

	nodeAPMCheckRegex = regexp.MustCompile(`"dd-trace"`)
)

// Detect attempts to detect the type of APM instrumentation for the given service.
func Detect(pid int, args []string, envs map[string]string, lang language.Language, contextMap usm.DetectorContextMap) Instrumentation {
	// first check to see if the DD_INJECTION_ENABLED is set to tracer
	if isInjected(envs) {
		return Injected
	}

	if _, ok := allowedLangs[lang]; !ok {
		return None
	}

	// different detection for provided instrumentation for each
	if detect, ok := detectorMap[lang]; ok {
		return detect(pid, args, envs, contextMap)
	}

	return None
}

func isInjected(envs map[string]string) bool {
	if val, ok := envs["DD_INJECTION_ENABLED"]; ok {
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
func goDetector(pid int, _ []string, _ map[string]string, _ usm.DetectorContextMap) Instrumentation {
	exePath := kernel.HostProc(strconv.Itoa(pid), "exe")

	elfFile, err := elf.Open(exePath)
	if err != nil {
		log.Debugf("Unable to open exe %s: %v", exePath, err)
		return None
	}
	defer elfFile.Close()

	_, err = bininspect.GetAnySymbolWithPrefix(elfFile, ddTraceGoPrefix, ddTraceGoMaxLength)
	if err != nil {
		return None
	}

	return Provided
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
func pythonDetector(pid int, _ []string, _ map[string]string, _ usm.DetectorContextMap) Instrumentation {
	mapsPath := kernel.HostProc(strconv.Itoa(pid), "maps")
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
func nodeDetector(_ int, _ []string, _ map[string]string, contextMap usm.DetectorContextMap) Instrumentation {
	pkgJSONPath, ok := contextMap[usm.NodePackageJSONPath]
	if !ok {
		log.Debugf("could not get package.json path from context map")
		return None
	}

	fs, ok := contextMap[usm.ServiceSubFS]
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

func javaDetector(_ int, args []string, envs map[string]string, _ usm.DetectorContextMap) Instrumentation {
	ignoreArgs := map[string]bool{
		"-version":     true,
		"-Xshare:dump": true,
		"/usr/share/ca-certificates-java/ca-certificates-java.jar": true,
	}

	// Check simple args on builtIn list.
	for _, v := range args {
		if ignoreArgs[v] {
			return None
		}
		// don't instrument if javaagent is already there on the command line
		if strings.HasPrefix(v, "-javaagent:") && strings.Contains(v, "dd-java-agent.jar") {
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
		if val, ok := envs[name]; ok {
			if strings.Contains(val, "-javaagent:") && strings.Contains(val, "dd-java-agent.jar") {
				return Provided
			}
		}
	}
	return None
}

func findFile(fileName string) (io.ReadCloser, bool) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, false
	}
	return f, true
}

const datadogDotNetInstrumented = "Datadog.Trace.ClrProfiler.Native"

func dotNetDetector(_ int, args []string, envs map[string]string, _ usm.DetectorContextMap) Instrumentation {
	// if it's just the word `dotnet` by itself, don't instrument
	if len(args) == 1 && args[0] == "dotnet" {
		return None
	}

	/*
			From Kevin Gosse:
			- CORECLR_ENABLE_PROFILING=1
		    - CORECLR_PROFILER_PATH environment variables set
		      (it means that a profiler is attached, it doesn't really matter if it's ours or another vendor)
	*/
	// don't instrument if the tracer is already installed
	foundFlags := 0
	if _, ok := envs["CORECLR_PROFILER_PATH"]; ok {
		foundFlags |= 1
	}
	if val, ok := envs["CORECLR_ENABLE_PROFILING"]; ok && val == "1" {
		foundFlags |= 2
	}
	if foundFlags == 3 {
		return Provided
	}

	ignoreArgs := map[string]bool{
		"build":   true,
		"clean":   true,
		"restore": true,
		"publish": true,
	}

	if len(args) > 1 {
		// Ignore only if the first arg match with the ignore list
		if ignoreArgs[args[1]] {
			return None
		}
		// Check to see if there's a DLL on the command line that contain the string Datadog.Trace.ClrProfiler.Native
		// If so, it's already instrumented with Datadog, ignore the process
		for _, v := range args[1:] {
			if strings.HasSuffix(v, ".dll") {
				if f, ok := findFile(v); ok {
					defer f.Close()
					offset, err := reader.Index(f, datadogDotNetInstrumented)
					if offset != -1 && err == nil {
						return Provided
					}
				}
			}
		}
	}

	// does the binary contain the string Datadog.Trace.ClrProfiler.Native (this should cover all single-file deployments)
	// if so, it's already instrumented with Datadog, ignore the process
	if f, ok := findFile(args[0]); ok {
		defer f.Close()
		offset, err := reader.Index(f, datadogDotNetInstrumented)
		if offset != -1 && err == nil {
			return Provided
		}
	}

	// check if there's a .dll in the directory with the same name as the binary used to launch it
	// if so, check if it has the Datadog.Trace.ClrProfiler.Native string
	// if so, it's already instrumented with Datadog, ignore the process
	if f, ok := findFile(args[0] + ".dll"); ok {
		defer f.Close()
		offset, err := reader.Index(f, datadogDotNetInstrumented)
		if offset != -1 && err == nil {
			return Provided
		}
	}

	// does the application folder contain the file Datadog.Trace.dll (this should cover "classic" deployments)
	// if so, it's already instrumented with Datadog, ignore the process
	if f, ok := findFile("Datadog.Trace.dll"); ok {
		f.Close()
		return Provided
	}
	return None
}
