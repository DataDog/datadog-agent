// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninsttest

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

//go:embed testdata/decoded
var testdataFS embed.FS

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
		for _, cfg := range cfgs {
			t.Run(fmt.Sprintf("%s-%s", svc, cfg), func(t *testing.T) {
				runIntegrationTestSuite(t, svc, cfg, rewrite, sem)
			})
		}
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
	expectedEventCounts := make(map[string]int)
	if !rewriteEnabled {
		for _, p := range probes {
			expectedEventCounts[p.GetID()] = len(expOut[p.GetID()])
		}
	}
	events, sink := waitForAttachmentAndCollectEvents(t, reporter, processInfo, eventCollectionConfig{
		RewriteEnabled:      rewriteEnabled,
		ExpectedEventCounts: expectedEventCounts,
		StartTime:           start,
	})
	if t.Failed() {
		return nil
	}
	_, err := processInfo.Process.Wait()
	require.NoError(t, err)
	cleanupProcess(t, processInfo, at, a)
	symbolicatorWrapper := createGoSymbolicator(t, servicePath)
	defer func() { require.NoError(t, symbolicatorWrapper.close()) }()
	return processAndDecodeEvents(t, events, sink, symbolicatorWrapper.Symbolicator, EventProcessingConfig{
		Service:        service,
		RewriteEnabled: rewriteEnabled,
		ExpectedOutput: expOut,
	})
}

type probeOutputs map[string][]json.RawMessage

func runIntegrationTestSuite(
	t *testing.T,
	service string,
	cfg testprogs.Config,
	rewrite bool,
	sem Semaphore,
) {
	RunIntegrationTestSuite(t, RunTestSuiteConfig{
		Service:   service,
		Config:    cfg,
		Rewrite:   rewrite,
		Semaphore: sem,
		TestFunc:  testDyninst,
	})
}

// validateAndSaveOutputs ensures that the outputs for the same probe are consistent
// across all tests and saves them to disk.
func validateAndSaveOutputs(
	t *testing.T, svc string, byTest map[string]probeOutputs,
) {
	byProbe := make(map[string][]byte)
	msgEq := func(a, b json.RawMessage) bool { return bytes.Equal(a, b) }
	findMismatchingTests := func(
		probeID string, cur []json.RawMessage,
	) (testNames []string) {
		for testName, testOutputs := range byTest {
			if out, ok := testOutputs[probeID]; ok {
				if !slices.EqualFunc(out, cur, msgEq) {
					testNames = append(testNames, testName)
				}
			}
		}
		return testNames
	}
	for testName, testOutputs := range byTest {
		for id, out := range testOutputs {
			marshaled, err := json.MarshalIndent(out, "", "  ")
			require.NoError(t, err)
			prev, ok := byProbe[id]
			if !ok {
				byProbe[id] = marshaled
				continue
			}
			if bytes.Equal(prev, marshaled) {
				continue
			}
			otherTestNames := findMismatchingTests(id, out)
			require.Equal(
				t,
				string(prev),
				string(marshaled),
				"inconsistent output for probe %s in test %s and %s",
				id, testName, strings.Join(otherTestNames, ", "),
			)
		}
	}
	for id, out := range byProbe {
		path := getProbeOutputFilename(svc, id)
		if err := saveActualOutputOfProbe(path, out); err != nil {
			t.Logf("error saving actual output for probe %s: %v", id, err)
		} else {
			t.Logf("output saved to: %s", path)
		}
	}
}

func getProbeOutputFilename(service, probeID string) string {
	return filepath.Join(
		"testdata", "decoded", service, probeID+".json",
	)
}

// getExpectedDecodedOutputOfProbes returns the expected output for a given service.
func getExpectedDecodedOutputOfProbes(progName string) (map[string][]json.RawMessage, error) {
	dir := filepath.Join("testdata", "decoded", progName)
	entries, err := testdataFS.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	expected := make(map[string][]json.RawMessage)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		probeID := strings.TrimSuffix(e.Name(), ".json")
		content, err := testdataFS.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		var out []json.RawMessage
		if err := json.Unmarshal(content, &out); err != nil {
			return nil, fmt.Errorf("unmarshalling %s: %w", e.Name(), err)
		}
		expected[probeID] = out
	}
	return expected, nil
}

// saveActualOutputOfProbes saves the actual output for a given service.
// The output is saved to the expected output directory with the same format as getExpectedDecodedOutputOfProbes.
// Note: This function now saves to the current working directory since embedded files are read-only.
func saveActualOutputOfProbe(outputPath string, content []byte) error {
	outputDir := path.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("error creating testdata directory: %w", err)
	}

	baseName := path.Base(outputPath)
	tmpFile, err := os.CreateTemp(outputDir, "."+baseName+".*.tmp.json")
	if err != nil {
		return fmt.Errorf("error creating temp output file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmpFile, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("error writing temp output: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("error closing temp output: %w", err)
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return fmt.Errorf("error renaming temp output: %w", err)
	}
	return nil
}
