// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package mode

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"

	"github.com/spf13/afero"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"

	"github.com/stretchr/testify/assert"
)

func TestBuildCommandParamWithArgs(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd", "--verbose", "path", "-i", "."})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{"--verbose", "path", "-i", "."}, args)
}

func TestBuildCommandParam(t *testing.T) {
	name, args := buildCommandParam([]string{"superCmd"})
	assert.Equal(t, "superCmd", name)
	assert.Equal(t, []string{}, args)
}

func TestPropagateChildSuccess(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit 0"})
		assert.Equal(t, nil, err)
	})
}

func TestPropagateChildError(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		expectedError := 123
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit " + strconv.Itoa(expectedError)})
		assert.Equal(t, expectedError<<8, int(err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)))
	})
}

func TestForwardSignalToChild(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		resultChan := make(chan error)
		terminatingSignal := syscall.SIGUSR1
		cmd := exec.Command("sleep", "2s")
		cmd.Start()
		sigs := make(chan os.Signal, 1)
		go forwardSignals(cmd.Process, sigs)

		go func() {
			err := cmd.Wait()
			resultChan <- err
		}()

		sigs <- syscall.SIGSTOP
		sigs <- syscall.SIGCONT
		sigs <- terminatingSignal

		err := <-resultChan
		assert.Equal(t, int(terminatingSignal), int(err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)))
	},
	)
}

func runTestOnLinuxOnly(t *testing.T, targetTest func(*testing.T)) {
	if runtime.GOOS == "linux" {
		t.Run("Test on Linux", func(t *testing.T) {
			targetTest(t)
		})
	} else {
		t.Skip("Test case is skipped on this platform")
	}
}

func TestNodeTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/node/")

	autoInstrumentTracer(fs)

	assert.Equal(t, "--require dd-trace/init", os.Getenv("NODE_OPTIONS"))
	assert.Equal(t, "/dd_tracer/node/", os.Getenv("NODE_PATH"))
}

func TestDotNetTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/dotnet/")

	autoInstrumentTracer(fs)

	assert.Equal(t, "1", os.Getenv("CORECLR_ENABLE_PROFILING"))
	assert.Equal(t, "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}", os.Getenv("CORECLR_PROFILER"))
	assert.Equal(t, "/dd_tracer/dotnet/Datadog.Trace.ClrProfiler.Native.so", os.Getenv("CORECLR_PROFILER_PATH"))
	assert.Equal(t, "/dd_tracer/dotnet/", os.Getenv("DD_DOTNET_TRACER_HOME"))
}

func TestJavaTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/java/")

	autoInstrumentTracer(fs)

	assert.Equal(t, "-javaagent:/dd_tracer/java/dd-java-agent.jar", os.Getenv("JAVA_TOOL_OPTIONS"))
}

func TestJavaTracerInstrumentationAddsSecondAgent(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/java/")

	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:some_agent.jar")

	autoInstrumentTracer(fs)

	assert.Equal(t, "-javaagent:some_agent.jar -javaagent:/dd_tracer/java/dd-java-agent.jar", os.Getenv("JAVA_TOOL_OPTIONS"))
}

func TestPythonTracerIsAutoInstrumented(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/python/")

	t.Setenv("PYTHONPATH", "/path1:/path2")

	autoInstrumentTracer(fs)

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
