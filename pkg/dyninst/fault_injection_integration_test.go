// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func injectMissingDecoderType(program *ir.Program, ev output.Event) error {
	for item, err := range ev.DataItems() {
		if err != nil {
			return err
		}
		_, ok := program.Types[ir.TypeID(item.Header().Type)].(*ir.EventRootType)
		if !ok {
			delete(program.Types, ir.TypeID(item.Header().Type))
		}
	}
	return nil
}

func testDyninstWithFaultInjection(
	t *testing.T,
	service string,
	servicePath string,
	probes []ir.ProbeDefinition,
	rewriteEnabled bool,
	expOut map[string][]json.RawMessage,
	debug bool,
	sem dyninsttest.Semaphore,
) map[string][]json.RawMessage {

	defer sem.Acquire()()
	start := time.Now()
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-integration-test")
	defer cleanup()

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, irDump.Close()) }()

	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, codeDump.Close()) }()

	loaderOpts := []loader.Option{
		loader.WithAdditionalSerializer(&compiler.DebugSerializer{
			Out: codeDump,
		}),
	}
	if debug {
		loaderOpts = append(loaderOpts, loader.WithDebugLevel(100))
	}
	reporter := makeTestReporter(t, irDump)
	loader, err := loader.NewLoader(loaderOpts...)
	require.NoError(t, err)
	a := actuator.NewActuator(loader)
	require.NoError(t, err)
	at := a.NewTenant("integration-test", reporter, irgen.NewGenerator())

	// Launch the sample service.
	t.Logf("launching %s", service)
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, servicePath,
	)
	defer func() {
		_ = sampleProc.Process.Kill()
		_ = sampleProc.Wait()
	}()

	stat, err := os.Stat(servicePath)
	require.NoError(t, err)
	fileInfo := stat.Sys().(*syscall.Stat_t)
	exe := actuator.Executable{
		Path: servicePath,
		Key: procmon.FileKey{
			FileHandle: procmon.FileHandle{
				Dev: uint64(fileInfo.Dev),
				Ino: fileInfo.Ino,
			},
		},
	}

	// Send update to actuator to instrument the process.
	at.HandleUpdate(actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{
			{
				ProcessID: actuator.ProcessID{
					PID: int32(sampleProc.Process.Pid),
				},
				Executable: exe,
				Probes:     probes,
			},
		},
		Removals: []actuator.ProcessID{},
	})

	// Wait for the process to be attached.
	t.Log("Waiting for attachment")
	sink, ok := <-reporter.attached
	require.True(t, ok)
	if t.Failed() {
		return nil
	}

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls")
	sampleStdin.Write([]byte("\n"))

	var totalExpectedEvents int
	if rewriteEnabled {
		totalExpectedEvents = math.MaxInt
	} else {
		for _, p := range probes {
			totalExpectedEvents += len(expOut[p.GetID()])
		}
	}

	timeout := time.Second
	if !rewriteEnabled {
		// In CI the machines seem to get very overloaded and this takes a
		// shocking amount of time. Given we don't wait for this timeout in
		// the happy path, it's fine to let this be quite long.
		timeout = 5*time.Second + 5*time.Since(start)
	}
	timeoutCh := time.After(timeout)
	var read []output.Event
	var timedOut bool
	for !timedOut && len(read) < totalExpectedEvents {
		select {
		case m := <-sink.ch:
			read = append(read, m)
		case <-timeoutCh:
			timedOut = true
		}
	}
	if !rewriteEnabled && timedOut {
		t.Errorf(
			"timed out after %v waiting for %d events, got %d",
			timeout, totalExpectedEvents, len(read),
		)
	}
	require.NoError(t, sampleProc.Wait())

	at.HandleUpdate(actuator.ProcessesUpdate{
		Removals: []actuator.ProcessID{
			{PID: int32(sampleProc.Process.Pid)},
		},
	})
	require.NoError(t, a.Shutdown())

	t.Logf("processing output")
	symbolicator := &faultySymbolicator{}
	decoder, err := decode.NewDecoder(sink.irp)
	require.NoError(t, err)

	retMap := make(map[string][]json.RawMessage)
	for _, ev := range read {
		// Validate that the header has the correct program ID.
		{
			header, err := ev.Header()
			require.NoError(t, err)
			require.Equal(t, ir.ProgramID(header.Prog_id), sink.irp.ID)
		}

		event := decode.Event{
			Event:       ev,
			ServiceName: service,
		}
		err = injectMissingDecoderType(sink.irp, ev)
		require.NoError(t, err)

		var (
			output []byte
			probe  ir.ProbeDefinition
			err    error
		)
		output, probe, err = decoder.Decode(event, symbolicator, output)
		require.NoError(t, err)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Output: %s", string(output))
		}
		redacted := redactJSON(t, "", output, defaultRedactors)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Sorted and redacted: %s", redacted)
		}
		probeID := probe.GetID()
		probeID = "faulty-" + probeID

		probeRet := retMap[probeID]
		expIdx := len(probeRet)
		retMap[probeID] = append(retMap[probeID], json.RawMessage(redacted))
		if !rewriteEnabled {
			expOut, ok := expOut[probeID]
			assert.True(t, ok, "expected output for probe %s not found", probeID)
			assert.Less(
				t, expIdx, len(expOut),
				"expected at least %d events for probe %s, got %d",
				expIdx+1, probeID, len(expOut),
			)
			assert.Equal(t, string(expOut[expIdx]), string(redacted))
		}
	}
	return retMap
}

func runIntegrationTestSuiteWithFaultInjection(
	t *testing.T,
	service string,
	cfg testprogs.Config,
	rewrite bool,
	sem dyninsttest.Semaphore,
) {
	if cfg.GOARCH != runtime.GOARCH {
		t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
		return
	}
	var outputs = struct {
		sync.Mutex
		byTest map[string]probeOutputs // testName -> probeID -> [redacted JSON]
	}{
		byTest: make(map[string]probeOutputs),
	}
	if rewrite {
		t.Cleanup(func() {
			if t.Failed() {
				return
			}
			validateAndSaveOutputs(t, service, outputs.byTest)
		})
	}
	probes := testprogs.MustGetProbeDefinitions(t, service)
	var expectedOutput map[string][]json.RawMessage
	if !rewrite {
		var err error
		expectedOutput, err = getExpectedDecodedOutputOfProbes(service)
		require.NoError(t, err)
	}
	bin := testprogs.MustGetBinary(t, service, cfg)
	for _, debug := range []bool{false, true} {
		runTest := func(t *testing.T, probeSlice []ir.ProbeDefinition) {
			t.Parallel()
			actual := testDyninstWithFaultInjection(
				t, service, bin, probeSlice, rewrite, expectedOutput, debug, sem,
			)
			if t.Failed() {
				return
			}
			outputs.Lock()
			defer outputs.Unlock()
			outputs.byTest[t.Name()] = actual
		}
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			if debug && testing.Short() {
				t.Skip("skipping debug with short")
			}
			t.Parallel()
			t.Run("all-probes", func(t *testing.T) { runTest(t, probes) })
			for i := range probes {
				probeID := probes[i].GetID()
				t.Run(probeID, func(t *testing.T) {
					if testing.Short() {
						t.Skip("skipping individual probe with short")
					}
					runTest(t, probes[i:i+1])
				})
			}
		})
	}
}

type faultySymbolicator struct {
}

func (s *faultySymbolicator) Symbolicate(_ []uint64, _ uint64) ([]symbol.StackFrame, error) {
	return nil, fmt.Errorf("error symbolicating stack")
}
