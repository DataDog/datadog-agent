// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package initcontainer

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/afero"
)

// Tracer holds a name, a path to the trace directory, and an
// initialization function that automatically instruments the
// tracer
type Tracer struct {
	FsPath string
	InitFn func()
}

func instrumentNode() {
	currNodePath := os.Getenv("NODE_PATH")
	os.Setenv("NODE_PATH", addToString(currNodePath, ":", "/dd_tracer/node/"))

	currNodeOptions := os.Getenv("NODE_OPTIONS")
	os.Setenv("NODE_OPTIONS", addToString(currNodeOptions, " ", "--require dd-trace/init"))
}

func instrumentJava() {
	currJavaToolOptions := os.Getenv("JAVA_TOOL_OPTIONS")
	os.Setenv("JAVA_TOOL_OPTIONS", addToString(currJavaToolOptions, " ", "-javaagent:/dd_tracer/java/dd-java-agent.jar"))
}

func instrumentDotnet() {
	os.Setenv("CORECLR_ENABLE_PROFILING", "1")
	os.Setenv("CORECLR_PROFILER", "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}")
	os.Setenv("CORECLR_PROFILER_PATH", "/dd_tracer/dotnet/Datadog.Trace.ClrProfiler.Native.so")
	os.Setenv("DD_DOTNET_TRACER_HOME", "/dd_tracer/dotnet/")
}

func instrumentPython() {
	os.Setenv("PYTHONPATH", addToString(os.Getenv("PYTHONPATH"), ":", "/dd_tracer/python/"))
}

// AutoInstrumentTracer searches the filesystem for a trace library, and
// automatically sets the correct environment variables.
func AutoInstrumentTracer(fs afero.Fs) {
	tracers := []Tracer{
		{"/dd_tracer/node/", instrumentNode},
		{"/dd_tracer/java/", instrumentJava},
		{"/dd_tracer/dotnet/", instrumentDotnet},
		{"/dd_tracer/python/", instrumentPython},
	}

	for _, tracer := range tracers {
		if ok, err := dirExists(fs, tracer.FsPath); ok {
			log.Debugf("Found %v, automatically instrumenting tracer", tracer.FsPath)
			os.Setenv("DD_TRACE_PROPAGATION_STYLE", "datadog")
			tracer.InitFn()
			return
		} else if err != nil {
			log.Debug("Error checking if directory exists: %v", err)
		}
	}
}

func dirExists(fs afero.Fs, path string) (bool, error) {
	_, err := fs.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func addToString(path string, separator string, token string) string {
	if path == "" {
		return token
	}

	return path + separator + token
}
