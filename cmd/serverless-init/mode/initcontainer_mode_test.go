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

// TestDetectMode_Sidecar_HasRunnerSet verifies that sidecar mode (no args)
// sets Runner to RunSidecar so main.go can call it directly.
func TestDetectMode_Sidecar_HasRunnerSet(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init"}

	conf := DetectMode()
	assert.True(t, conf.SidecarMode)
	assert.NotNil(t, conf.Runner, "sidecar mode must set Runner to RunSidecar")
}

// TestDetectMode_Init_RunnerIsNil verifies that init mode (args present) leaves
// Runner nil. main.go delegates to cloudService.Run(modeConf, logConfig) which
// each CloudService implements directly, so modeConf.Runner is not used in
// init-container mode.
func TestDetectMode_Init_RunnerIsNil(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "exit 0"}

	conf := DetectMode()
	assert.False(t, conf.SidecarMode)
	assert.Nil(t, conf.Runner, "init mode Runner must be nil; main.go builds the closure with child")
}

// TestHandleTerminationSignals_SIGTERM and _SIGINT exercise handleTerminationSignals
// via its injectable notify parameter, avoiding real OS signals and verifying
// that either signal unblocks stopCh.
func TestHandleTerminationSignals_SIGTERM(t *testing.T) {
	stopCh := make(chan struct{}, 1)
	notify := func(c chan<- os.Signal, _ ...os.Signal) {
		go func() { c <- syscall.SIGTERM }()
	}
	handleTerminationSignals(stopCh, notify)
	select {
	case <-stopCh:
	default:
		t.Fatal("stopCh must be closed after SIGTERM")
	}
}

func TestHandleTerminationSignals_SIGINT(t *testing.T) {
	stopCh := make(chan struct{}, 1)
	notify := func(c chan<- os.Signal, _ ...os.Signal) {
		go func() { c <- syscall.SIGINT }()
	}
	handleTerminationSignals(stopCh, notify)
	select {
	case <-stopCh:
	default:
		t.Fatal("stopCh must be closed after SIGINT")
	}
}

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

// When cmd.Start fails (e.g. binary not found), execute must return the error
// and never invoke OnAlive — the user app never ran.
func TestExecute_StartFailure_NeverCallsOnAlive(t *testing.T) {
	var onAliveCalled bool
	hooks := &ProcessHooks{
		OnAlive: func() { onAliveCalled = true },
		OnDead:  func() {},
	}
	err := execute(&serverlessLog.Config{}, []string{"/nonexistent/binary/that/cannot/be/found"}, hooks)
	assert.Error(t, err, "cmd.Start must fail for a missing binary")
	assert.False(t, onAliveCalled, "OnAlive must not be called when cmd.Start fails")
}

// On a successful run, execute must call OnAlive after cmd.Start and OnDead
// via defer after cmd.Wait. The mid-run probe pins the ordering.
func TestExecute_SuccessfulRun_InvokesHooksInOrder(t *testing.T) {
	child := lifecycle.NewChild()
	hooks := &ProcessHooks{
		OnAlive: child.MarkAlive,
		OnDead:  child.MarkDead,
	}
	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()
	err := execute(&serverlessLog.Config{}, []string{"sh", "-c", "sleep 0.5"}, hooks)
	<-probeDone
	assert.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "OnAlive must fire before cmd.Wait returns")
	assert.False(t, child.IsAlive(), "OnDead must fire after cmd.Wait returns")
}

// MicroVM init-container mode: ProcessHooks drive liveness tracking through
// the public RunInit entry point. Pins the alive→dead transition.
func TestRunInit_MicroVM_ChildSupplied_TracksLiveness(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "sleep 0.5"}

	child := lifecycle.NewChild()
	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()
	err := RunInit(&serverlessLog.Config{}, &ProcessHooks{OnAlive: child.MarkAlive, OnDead: child.MarkDead})
	<-probeDone
	assert.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "child must be marked alive while RunInit is blocked")
	assert.False(t, child.IsAlive(), "child must be marked dead after RunInit returns")
}

// Non-MicroVM mode: nil hooks. RunInit must execute the user app without
// panicking. Pins the `if hooks != nil` guard.
func TestRunInit_NonMicroVM_NoHooks_StillExecutes(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "exit 0"}

	err := RunInit(&serverlessLog.Config{}, nil)
	assert.NoError(t, err)
}

// On a non-zero exit, OnDead must still fire. Pins the defer-on-error path.
func TestExecute_NonZeroExit_OnDeadFiresAfterExit(t *testing.T) {
	child := lifecycle.NewChild()
	// Simulate MicroVM wiring: OnAlive fires first so child is alive, then
	// OnDead fires on defer.
	hooks := &ProcessHooks{OnAlive: child.MarkAlive, OnDead: child.MarkDead}
	err := execute(&serverlessLog.Config{}, []string{"sh", "-c", "exit 7"}, hooks)
	assert.Error(t, err, "non-zero exit must surface as an error")
	assert.False(t, child.IsAlive(), "OnDead must fire after non-zero exit")
}

// ProcessHooks with OnAlive set but OnDead nil must not panic. Pins the
// nil-guard on hooks.OnDead added to execute().
func TestExecute_OnAliveSetOnDeadNil_DoesNotPanic(t *testing.T) {
	hooks := &ProcessHooks{
		OnAlive: func() {},
		OnDead:  nil, // intentionally absent
	}
	assert.NotPanics(t, func() {
		_ = execute(&serverlessLog.Config{}, []string{"sh", "-c", "exit 0"}, hooks)
	})
}

// An OnDead-only hook (OnAlive nil) must still fire. Regression test for a bug
// where OnDead's defer was registered inside the `hooks.OnAlive != nil` branch,
// so it never ran without an OnAlive hook.
func TestExecute_OnDeadOnly_OnAliveNil_StillFires(t *testing.T) {
	child := lifecycle.NewChild()
	child.MarkAlive()
	hooks := &ProcessHooks{OnAlive: nil, OnDead: child.MarkDead}
	err := execute(&serverlessLog.Config{}, []string{"sh", "-c", "exit 0"}, hooks)
	assert.NoError(t, err)
	assert.False(t, child.IsAlive(), "OnDead must fire even when OnAlive is nil")
}

// OnDead must still fire if OnAlive panics. Regression test for a bug where
// OnDead's defer was registered only after hooks.OnAlive() returned, so a
// panic in OnAlive skipped it despite the doc comment promising otherwise.
func TestExecute_OnAlivePanics_OnDeadStillFires(t *testing.T) {
	child := lifecycle.NewChild()
	child.MarkAlive()
	hooks := &ProcessHooks{
		OnAlive: func() { panic("boom") },
		OnDead:  child.MarkDead,
	}
	func() {
		defer func() { recover() }()
		_ = execute(&serverlessLog.Config{}, []string{"sh", "-c", "exit 0"}, hooks)
	}()
	assert.False(t, child.IsAlive(), "OnDead must fire even if OnAlive panics")
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

func TestDotNetTracerInstrumentationRespectsCustomerValues(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/dotnet/")

	t.Setenv("CORECLR_ENABLE_PROFILING", "0")
	t.Setenv("CORECLR_PROFILER", "{01234567-89AB-CDEF-0123-456789ABCDEF}")
	t.Setenv("CORECLR_PROFILER_PATH", "/custom/profiler.so")
	t.Setenv("DD_DOTNET_TRACER_HOME", "/custom/tracer/home/")

	autoInstrumentTracer(fs)

	assert.Equal(t, "0", os.Getenv("CORECLR_ENABLE_PROFILING"))
	assert.Equal(t, "{01234567-89AB-CDEF-0123-456789ABCDEF}", os.Getenv("CORECLR_PROFILER"))
	assert.Equal(t, "/custom/profiler.so", os.Getenv("CORECLR_PROFILER_PATH"))
	assert.Equal(t, "/custom/tracer/home/", os.Getenv("DD_DOTNET_TRACER_HOME"))
}

func TestAutoInstrumentDoesNotSetPropagationStyle(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Create("/dd_tracer/python/")

	autoInstrumentTracer(fs)

	_, set := os.LookupEnv("DD_TRACE_PROPAGATION_STYLE")
	assert.False(t, set, "auto-instrumentation should leave DD_TRACE_PROPAGATION_STYLE to the tracer default")
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
