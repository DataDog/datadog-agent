// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package initcontainer

import (
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestNodeTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/node/")

	AutoInstrumentTracer(fs)

	assert.Equal(t, "--require dd-trace/init", os.Getenv("NODE_OPTIONS"))
	assert.Equal(t, "/dd_tracer/node/", os.Getenv("NODE_PATH"))
}

func TestDotNetTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/dotnet/")

	AutoInstrumentTracer(fs)

	assert.Equal(t, "1", os.Getenv("CORECLR_ENABLE_PROFILING"))
	assert.Equal(t, "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}", os.Getenv("CORECLR_PROFILER"))
	assert.Equal(t, "/dd_tracer/dotnet/Datadog.Trace.ClrProfiler.Native.so", os.Getenv("CORECLR_PROFILER_PATH"))
	assert.Equal(t, "/dd_tracer/dotnet/", os.Getenv("DD_DOTNET_TRACER_HOME"))
}

func TestJavaTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/java/")

	AutoInstrumentTracer(fs)

	assert.Equal(t, "-javaagent:/dd_tracer/java/dd-java-agent.jar", os.Getenv("JAVA_TOOL_OPTIONS"))
}

func TestJavaTracerInstrumentationAddsSecondAgent(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/java/")

	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:some_agent.jar")

	AutoInstrumentTracer(fs)

	assert.Equal(t, "-javaagent:some_agent.jar -javaagent:/dd_tracer/java/dd-java-agent.jar", os.Getenv("JAVA_TOOL_OPTIONS"))
}

func TestPythonTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/python/")

	t.Setenv("PYTHONPATH", "/path1:/path2")

	AutoInstrumentTracer(fs)

	assert.Equal(t, "/path1:/path2:/dd_tracer/python/", os.Getenv("PYTHONPATH"))
}

func TestAddToString(t *testing.T) {
	oldStr := "123"
	assert.Equal(t, "1234", addToString(oldStr, "", "4"))

	oldStr = ""
	assert.Equal(t, "", addToString(oldStr, "", ""))

	oldStr = "0"
	assert.Equal(t, "0:1", addToString(oldStr, ":", "1"))
}
