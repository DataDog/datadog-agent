// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninsttest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestDyninst(t *testing.T) {
	SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	programs := testprogs.MustGetPrograms(t)
	var integrationTestPrograms = map[string]struct{}{
		"simple": {},
		"sample": {},
	}

	sem := MakeSemaphore()

	// The debug variants of the tests spew logs to the trace_pipe, so we need
	// to clear it after the tests to avoid interfering with other tests.
	// Leave the option to disable this behavior for debugging purposes.
	dontClear, _ := strconv.ParseBool(os.Getenv("DONT_CLEAR_TRACE_PIPE"))
	if !dontClear {
		t.Logf("clearing trace_pipe!")
		tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
		require.NoError(t, err)
		t.Cleanup(func() {
			for {
				deadline := time.Now().Add(100 * time.Millisecond)
				require.NoError(t, tp.SetReadDeadline(deadline))
				n, err := io.Copy(io.Discard, tp)
				require.ErrorIs(t, err, os.ErrDeadlineExceeded)
				if n == 0 {
					break
				}
			}
			t.Logf("closing trace_pipe!")
			require.NoError(t, tp.Close())
		})
	}
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	for _, svc := range programs {
		if _, ok := integrationTestPrograms[svc]; !ok {
			t.Logf("%s is not used in integration tests", svc)
			continue
		}
		t.Run(svc, func(t *testing.T) {
			runIntegrationTestSuite(t, svc, rewrite, sem, cfgs...)
		})
	}
}

func testDyninst(
	t *testing.T,
	service string,
	servicePath string,
	probes []ir.ProbeDefinition,
	rewriteEnabled bool,
	expOut map[string][]json.RawMessage,
	debug bool,
	sem Semaphore,
) map[string][]json.RawMessage {
	defer sem.Acquire()()
	start := time.Now()
	env := prepareTestEnvironment(t, "dyninst-integration-test")
	defer env.Cleanup()

	a, at, reporter := createActuatorWithTenant(t, env, actuatorConfig{Debug: debug})

	ctx := context.Background()
	processInfo := launchTestProcess(ctx, t, env, service, servicePath)
	defer func() {
		_ = processInfo.Process.Kill()
		_, _ = processInfo.Process.Wait()
	}()

	instrumentProcess(at, processInfo, probes)

	// Prepare expected event counts
	expectedEventCounts := make(map[string]int)
	if !rewriteEnabled {
		for _, p := range probes {
			expectedEventCounts[p.GetID()] = len(expOut[p.GetID()])
		}
	}

	read, sink := waitForAttachmentAndCollectEvents(t, reporter, processInfo, eventCollectionConfig{
		RewriteEnabled:      rewriteEnabled,
		ExpectedEventCounts: expectedEventCounts,
		StartTime:           start,
	})
	if t.Failed() {
		return nil
	}
	var err error
	_, err = processInfo.Process.Wait()
	require.NoError(t, err)

	cleanupProcess(t, processInfo, at, a)

	g := createGoSymbolicator(t, servicePath)
	defer func() { require.NoError(t, g.close()) }()

	return processAndDecodeEvents(t, read, sink, g.Symbolicator, EventProcessingConfig{
		Service:        service,
		RewriteEnabled: rewriteEnabled,
		ExpectedOutput: expOut,
	})
}

func runIntegrationTestSuite(
	t *testing.T,
	service string,
	rewrite bool,
	sem Semaphore,
	cfgs ...testprogs.Config,
) {
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
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
				return
			}
			t.Parallel()
			bin := testprogs.MustGetBinary(t, service, cfg)
			for _, debug := range []bool{false, true} {
				runTest := func(t *testing.T, probeSlice []ir.ProbeDefinition) {
					t.Parallel()
					actual := testDyninst(
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
		})
	}
}
