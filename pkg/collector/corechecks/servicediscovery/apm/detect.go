// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apm provides functionality to detect the type of APM instrumentation a service is using.
package apm

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language/reader"
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

type detector func(args []string, envs map[string]string) Instrumentation

var (
	detectorMap = map[language.Language]detector{
		language.DotNet: dotNetDetector,
		language.Java:   javaDetector,
		language.Node:   nodeDetector,
		language.Python: pythonDetector,
		language.Ruby:   rubyDetector,
	}
	// For now, only allow a subset of the above detectors to actually run.
	allowedLangs = map[language.Language]struct{}{
		language.Java: {},
		language.Node: {},
	}

	nodeAPMCheckRegex = regexp.MustCompile(`"dd-trace"`)
)

// Detect attempts to detect the type of APM instrumentation for the given service.
func Detect(args []string, envs map[string]string, lang language.Language) Instrumentation {
	// first check to see if the DD_INJECTION_ENABLED is set to tracer
	if isInjected(envs) {
		return Injected
	}

	if _, ok := allowedLangs[lang]; !ok {
		return None
	}

	// different detection for provided instrumentation for each
	if detect, ok := detectorMap[lang]; ok {
		return detect(args, envs)
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

func rubyDetector(_ []string, _ map[string]string) Instrumentation {
	return None
}

func pythonDetector(args []string, envs map[string]string) Instrumentation {
	/*
		Check for VIRTUAL_ENV env var
			if it's there, use $VIRTUAL_ENV/lib/python{}/site-packages/ and see if ddtrace is inside
			if so, return PROVIDED
			if it's not there,
				exec args[0] -c "import sys; print(':'.join(sys.path))"
				split on :
				for each part
					see if it ends in site-packages
					if so, check if ddtrace is inside
						if so, return PROVIDED
			return NONE
	*/
	if path, ok := envs["VIRTUAL_ENV"]; ok {
		venv := os.DirFS(path)
		libContents, err := fs.ReadDir(venv, "lib")
		if err == nil {
			for _, v := range libContents {
				if strings.HasPrefix(v.Name(), "python") && v.IsDir() {
					tracedir, err := fs.Stat(venv, "lib/"+v.Name()+"/site-packages/ddtrace")
					if err == nil && tracedir.IsDir() {
						return Provided
					}
				}
			}
		}
		// the virtual env didn't have ddtrace, can exit
		return None
	}
	// slow option...
	results, err := exec.Command(args[0], `-c`, `"import sys; print(':'.join(sys.path))"`).Output()
	if err != nil {
		log.Warn("Failed to execute command", err)
		return None
	}

	results = bytes.TrimSpace(results)
	parts := strings.Split(string(results), ":")
	for _, v := range parts {
		if strings.HasSuffix(v, "/site-packages") {
			_, err := os.Stat(v + "/ddtrace")
			if err == nil {
				return Provided
			}
		}
	}
	return None
}

// isNodeInstrumented parses the provided `os.File` trying to find an
// entry for APM NodeJS instrumentation. Returns true if finding such
// an entry, false otherwise.
func isNodeInstrumented(f *os.File) bool {
	const readLimit = 16 * 1024 // Read 16KiB max

	limitReader := io.LimitReader(f, readLimit)
	bufferedReader := bufio.NewReader(limitReader)

	return nodeAPMCheckRegex.MatchReader(bufferedReader)
}

// nodeDetector checks if a service has APM NodeJS instrumentation.
//
// To check for APM instrumentation, we try to find a package.json in
// the parent directories of the service. If found, we then check for a
// `dd-trace` entry to be present.
func nodeDetector(_ []string, envs map[string]string) Instrumentation {
	processWorkingDirectory := ""
	if val, ok := envs["PWD"]; ok {
		processWorkingDirectory = filepath.Clean(val)
	} else {
		log.Debug("unable to determine working directory, assuming uninstrumented")
		return None
	}

	for curDir := processWorkingDirectory; len(curDir) > 1; curDir = filepath.Dir(curDir) {
		pkgJSONPath := filepath.Join(curDir, "package.json")
		pkgJSONFile, err := os.Open(pkgJSONPath)
		if err != nil {
			log.Debugf("could not open package.json: %s", err)
			continue
		}
		log.Debugf("found package.json: %s", pkgJSONPath)

		isInstrumented := isNodeInstrumented(pkgJSONFile)
		_ = pkgJSONFile.Close()

		if isInstrumented {
			return Provided
		}
	}

	return None
}

func javaDetector(args []string, envs map[string]string) Instrumentation {
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

func dotNetDetector(args []string, envs map[string]string) Instrumentation {
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
