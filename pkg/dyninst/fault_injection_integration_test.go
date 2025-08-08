// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

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

	// Set up test environment
	env := prepareTestEnvironment(t, "dyninst-integration-test")
	defer env.Cleanup()

	// Create actuator and tenant
	a, at, reporter := createActuatorWithTenant(t, env, actuatorConfig{Debug: debug})

	// Launch test process
	ctx := context.Background()
	processInfo := launchTestProcess(ctx, t, env, service, servicePath)
	defer func() {
		_ = processInfo.Process.Kill()
		_, _ = processInfo.Process.Wait()
	}()

	// Instrument the process
	instrumentProcess(at, processInfo, probes)

	// Prepare expected event counts for collection
	expectedEventCounts := make(map[string]int)
	if !rewriteEnabled {
		for _, p := range probes {
			expectedEventCounts[p.GetID()] = len(expOut[p.GetID()])
		}
	}

	// Collect events
	events, sink := waitForAttachmentAndCollectEvents(t, reporter, processInfo, eventCollectionConfig{
		RewriteEnabled:      rewriteEnabled,
		ExpectedEventCounts: expectedEventCounts,
		StartTime:           start,
	})
	if t.Failed() {
		return nil
	}

	// Wait for process to finish (must happen before event processing for symbolication to work)
	_, err := processInfo.Process.Wait()
	require.NoError(t, err)

	// Clean up actuator
	cleanupProcess(t, processInfo, at, a)

	// Process events with fault injection
	return processAndDecodeEventsWithFaultInjection(t, events, sink, EventProcessingConfig{
		Service:        service,
		RewriteEnabled: rewriteEnabled,
		ExpectedOutput: expOut,
		ProbeIDPrefix:  "faulty-",
	})
}

func runIntegrationTestSuiteWithFaultInjection(
	t *testing.T,
	service string,
	cfg testprogs.Config,
	rewrite bool,
	sem dyninsttest.Semaphore,
) {
	RunIntegrationTestSuite(t, RunTestSuiteConfig{
		Service:        service,
		Config:         cfg,
		Rewrite:        rewrite,
		Semaphore:      sem,
		TestNameSuffix: "-fault-injection",
		TestFunc:       testDyninstWithFaultInjection,
	})
}

// faultySymbolicator has been moved to test_utilities.go as FaultySymbolicator
