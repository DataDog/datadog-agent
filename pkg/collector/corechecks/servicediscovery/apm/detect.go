// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apm provides functionality to detect the type of APM instrumentation a service is using.
package apm

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language/reader"
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

type detector func(logger *zap.Logger, args []string, envs []string) Instrumentation

var (
	detectorMap = map[language.Language]detector{
		language.DotNet: dotNetDetector,
		language.Java:   javaDetector,
		language.Node:   nodeDetector,
		language.Python: pythonDetector,
		language.Ruby:   rubyDetector,
	}
)

// Detect attempts to detect the type of APM instrumentation for the given service.
func Detect(logger *zap.Logger, args []string, envs []string, lang language.Language) Instrumentation {
	// first check to see if the DD_INJECTION_ENABLED is set to tracer
	if isInjected(envs) {
		return Injected
	}

	// different detection for provided instrumentation for each
	if detect, ok := detectorMap[lang]; ok {
		return detect(logger, args, envs)
	}

	return None
}

func isInjected(envs []string) bool {
	for _, env := range envs {
		if !strings.HasPrefix(env, "DD_INJECTION_ENABLED=") {
			continue
		}
		_, val, _ := strings.Cut(env, "=")
		parts := strings.Split(val, ",")
		for _, v := range parts {
			if v == "tracer" {
				return true
			}
		}
	}
	return false
}

func rubyDetector(_ *zap.Logger, _ []string, _ []string) Instrumentation {
	return None
}

func pythonDetector(logger *zap.Logger, args []string, envs []string) Instrumentation {
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
	for _, env := range envs {
		if strings.HasPrefix(env, "VIRTUAL_ENV=") {
			_, path, _ := strings.Cut(env, "=")
			venv := os.DirFS(path)
			libContents, err := fs.ReadDir(venv, "lib")
			if err != nil {
				continue
			}
			for _, v := range libContents {
				if strings.HasPrefix(v.Name(), "python") && v.IsDir() {
					tracedir, err := fs.Stat(venv, "lib/"+v.Name()+"/site-packages/ddtrace")
					if err != nil {
						continue
					}
					if tracedir.IsDir() {
						return Provided
					}
				}
			}
			// the virtual env didn't have ddtrace, can exit
			return None
		}
	}
	// slow option...
	results, err := exec.Command(args[0], `-c`, `"import sys; print(':'.join(sys.path))"`).Output()
	if err != nil {
		logger.Warn("Failed to execute command", zap.Error(err))
		return None
	}

	results = bytes.TrimSpace(results)
	parts := strings.Split(string(results), ":")
	logger.Debug("parts", zap.Strings("parts", parts))
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

func nodeDetector(logger *zap.Logger, _ []string, envs []string) Instrumentation {
	// check package.json, see if it has dd-trace in it.
	// first find it
	wd := ""
	for _, v := range envs {
		if strings.HasPrefix(v, "PWD=") {
			_, wd, _ = strings.Cut(v, "=")
			break
		}
	}
	if wd == "" {
		// don't know the working directory, just quit
		logger.Debug("unable to determine working directory, assuming uninstrumented")
		return None
	}

	// find package.json, see if already instrumented
	// whatever is the first package.json that we find, we use
	// we keep checking up to the root of the file system
	for curWD := filepath.Clean(wd); len(curWD) > 1; curWD = filepath.Dir(curWD) {
		curPkgJSON := curWD + string(filepath.Separator) + "package.json"
		f, err := os.Open(curPkgJSON)
		// this error means the file isn't there, so check parent directory
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				logger.Debug("package.json not found", zap.String("path", curPkgJSON))
			} else {
				logger.Debug("error opening package.json", zap.String("path", curPkgJSON), zap.Error(err))
			}
			continue
		}
		offset, err := reader.Index(f, `"dd-trace"`)
		if err != nil {
			logger.Debug("error reading package.json", zap.String("path", curPkgJSON), zap.Error(err))
			_ = f.Close()
			continue
		}
		if offset != -1 {
			_ = f.Close()
			return Provided
		}
		// intentionally ignoring error here
		_ = f.Close()
		return None
	}
	return None
}

func javaDetector(_ *zap.Logger, args []string, envs []string) Instrumentation {
	ignoreArgs := map[string]bool{
		"-version":     true,
		"-Xshare:dump": true,
		"/usr/share/ca-certificates-java/ca-certificates-java.jar": true,
	}

	//Check simple args on builtIn list.
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
	toolOptionEnvs := map[string]bool{
		// These are the environment variables that are used to pass options to the JVM
		"JAVA_TOOL_OPTIONS": true,
		"_JAVA_OPTIONS":     true,
		"JDK_JAVA_OPTIONS":  true,
		// I'm pretty sure these won't be necessary, as they should be parsed before the JVM sees them
		// but there's no harm in including them
		"JAVA_OPTIONS":  true,
		"CATALINA_OPTS": true,
		"JDPA_OPTS":     true,
	}
	for _, v := range envs {
		name, val, _ := strings.Cut(v, "=")
		if toolOptionEnvs[name] {
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

func dotNetDetector(_ *zap.Logger, args []string, envs []string) Instrumentation {
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
	for _, v := range envs {
		if strings.HasPrefix(v, "CORECLR_PROFILER_PATH") {
			foundFlags |= 1
		}
		if v == "CORECLR_ENABLE_PROFILING=1" {
			foundFlags |= 2
		}
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
