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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
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
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit 0"}, nil)
		assert.Equal(t, nil, err)
	})
}

func TestPropagateChildError(t *testing.T) {
	runTestOnLinuxOnly(t, func(t *testing.T) {
		expectedError := 123
		err := execute(&serverlessLog.Config{}, []string{"bash", "-c", "exit " + strconv.Itoa(expectedError)}, nil)
		assert.Equal(t, expectedError<<8, int(err.(*exec.ExitError).ProcessState.Sys().(syscall.WaitStatus)))
	})
}

// When cmd.Start fails (e.g. binary not found), execute must return the
// error and leave the ChildHandle in the not-alive state — the user app
// never ran, so /ready must keep returning 503.
func TestExecute_StartFailure_LeavesChildNotAlive(t *testing.T) {
	child := lifecycle.NewChild()
	err := execute(&serverlessLog.Config{}, []string{"/nonexistent/binary/that/cannot/be/found"}, child)
	assert.Error(t, err, "cmd.Start must fail for a missing binary")
	assert.False(t, child.IsAlive(), "child must remain not-alive when cmd.Start fails")
}

// On a successful run, execute must mark the child alive between cmd.Start
// and cmd.Wait, then mark it dead via the deferred MarkDead once cmd.Wait
// returns. The mid-run probe pins the alive→dead ordering — without it, a
// buggy implementation that skipped MarkAlive entirely would still pass a
// post-run-only assertion.
func TestExecute_SuccessfulRun_MarksChildAliveThenDead(t *testing.T) {
	child := lifecycle.NewChild()
	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		// Generous offset (~10× fork+exec time) so MarkAlive has fired.
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()
	err := execute(&serverlessLog.Config{}, []string{"sh", "-c", "sleep 0.5"}, child)
	<-probeDone
	assert.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "child must be marked alive while cmd.Wait is blocked")
	assert.False(t, child.IsAlive(), "child must be marked dead after cmd.Wait returns")
}

// MicroVM init-container mode: RunInit is given a non-nil *lifecycle.Child
// and must thread it through execute so /ready can observe liveness. The
// mid-run probe pins both transitions through the public entry point —
// IsAlive=true while the child is running, IsAlive=false after RunInit
// returns (deferred MarkDead).
func TestRunInit_MicroVM_ChildSupplied_TracksLiveness(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "sleep 0.5"}

	child := lifecycle.NewChild()
	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		// Generous offset (~10× fork+exec time) so MarkAlive has fired.
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()
	err := RunInit(&serverlessLog.Config{}, child)
	<-probeDone
	assert.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "child must be marked alive while RunInit is blocked on the child")
	assert.False(t, child.IsAlive(), "child must be marked dead after RunInit returns")
}

// Non-MicroVM mode: child is nil. RunInit must still execute the user app
// without panicking on the nil guard. Pins the `if child != nil` branch.
func TestRunInit_NonMicroVM_NoChild_StillExecutes(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "exit 0"}

	err := RunInit(&serverlessLog.Config{}, nil)
	assert.NoError(t, err)
}

// On a non-zero exit, the deferred MarkDead must still fire so /ready
// reflects reality. Pins the defer-on-error path.
func TestExecute_NonZeroExit_MarksChildDeadAfterExit(t *testing.T) {
	child := lifecycle.NewChild()
	err := execute(&serverlessLog.Config{}, []string{"sh", "-c", "exit 7"}, child)
	assert.Error(t, err, "non-zero exit must surface as an error")
	assert.False(t, child.IsAlive(), "child must be marked dead after non-zero exit")
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
	assert.Equal(t, "/dd_tracer/node/:/dd_tracer/node/node_modules", os.Getenv("NODE_PATH"))
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
