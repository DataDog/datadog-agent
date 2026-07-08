// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// TestStopFlushesAllWorkersWhenEnabled verifies the flush-on-stop behavior:
// when dogstatsd_flush_incomplete_buckets is set, stop() must drain every
// worker's batched samples into the time sampler before returning. Each worker
// gets a uniquely-named sample so the assertion verifies EVERY worker drained,
// not just a total count — a regression where one fast worker double-fires
// while a busy peer never flushes would match the total but miss one of the
// unique names. This is the serverless-init production path
// (newServerlessBatcher) the deleted ServerlessFlush method used to cover.
func TestStopFlushesAllWorkersWhenEnabled(t *testing.T) {
	const workerCount = 2

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_workers_count"] = workerCount
	cfg["dogstatsd_flush_incomplete_buckets"] = true

	// Serverless-mode helper so workers run newServerlessBatcher — the exact
	// production code path exercised by serverless-init.
	deps := fulfillDepsWithServerlessConfigOverride(t, cfg)
	s := deps.Server.(*dsdServer)
	requireStart(t, s)
	require.Len(t, s.workers, workerCount, "expected the configured number of workers")

	// Workers are parked on select with no packetsIn traffic, so direct batcher
	// writes from this test goroutine are safe. stop() closes stopChan and then
	// waits on workerWg, so each worker's batcher.flush() (run in its stopChan
	// case) has completed by the time stop() returns.
	expectedNames := make(map[string]struct{}, workerCount)
	for i, w := range s.workers {
		name := "serverless.flush.test.worker." + strconv.Itoa(i)
		expectedNames[name] = struct{}{}
		w.batcher.appendSample(metrics.MetricSample{
			Name:       name,
			Value:      float64(i + 1),
			Mtype:      metrics.GaugeType,
			SampleRate: 1,
		})
	}

	require.NoError(t, s.stop(context.Background()))
	requireStopped(t, s)

	// Wait for at least workerCount samples, then verify each unique per-worker
	// name appears — proves every worker's batcher.flush() actually ran.
	samples, _ := deps.Demultiplexer.WaitForNumberOfSamples(workerCount, 0, 2*time.Second)
	gotNames := make(map[string]struct{}, len(samples))
	for _, sample := range samples {
		gotNames[sample.Name] = struct{}{}
	}
	for name := range expectedNames {
		assert.Contains(t, gotNames, name, "expected sample from each worker — fan-out regression: missing %s", name)
	}
}

// TestStopDrainsQueuedPacketsWhenEnabled verifies that flush-on-stop also
// rescues packets still queued in packetsIn but not yet parsed. Stopping the
// listeners can leave assembled packets in packetsIn, and the random select in
// worker.run could otherwise let a worker exit on stopChan before parsing them,
// dropping the metrics. drainAndFlush parses the remaining queue before flushing.
func TestStopDrainsQueuedPacketsWhenEnabled(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_workers_count"] = 1
	cfg["dogstatsd_flush_incomplete_buckets"] = true

	deps := fulfillDepsWithServerlessConfigOverride(t, cfg)
	s := deps.Server.(*dsdServer)
	requireStart(t, s)
	require.Len(t, s.workers, 1)

	// Stop the run loop first so this goroutine can own packetsIn without racing
	// the worker, then drive drainAndFlush directly — the exact code path the
	// stopChan case takes when packets are still queued at stop. stop() leaves
	// packetsIn open, so we can enqueue afterwards.
	require.NoError(t, s.stop(context.Background()))
	requireStopped(t, s)

	w := s.workers[0]
	s.packetsIn <- genTestPackets([]byte("drain.test.metric:42|g"))
	w.drainAndFlush()

	assert.Empty(t, s.packetsIn, "drainAndFlush must consume every queued packet before returning")

	samples, _ := deps.Demultiplexer.WaitForNumberOfSamples(1, 0, 2*time.Second)
	require.Len(t, samples, 1, "the queued packet must be parsed and flushed to the sampler")
	assert.Equal(t, "drain.test.metric", samples[0].Name)
}

// TestStopDoesNotFlushWhenDisabled verifies the gate is honored: with
// dogstatsd_flush_incomplete_buckets unset (the long-running-agent default),
// stop() must NOT drain worker batchers — workers just exit on stopChan. The
// batched sample must therefore never reach the sampler.
func TestStopDoesNotFlushWhenDisabled(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_workers_count"] = 1
	// dogstatsd_flush_incomplete_buckets intentionally left unset (defaults to false).

	deps := fulfillDepsWithServerlessConfigOverride(t, cfg)
	s := deps.Server.(*dsdServer)
	requireStart(t, s)
	require.Len(t, s.workers, 1)

	s.workers[0].batcher.appendSample(metrics.MetricSample{
		Name:       "serverless.noflush.test",
		Value:      1,
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	})

	require.NoError(t, s.stop(context.Background()))
	requireStopped(t, s)

	// No flush was triggered, so no sample should ever arrive. WaitForNumberOfSamples
	// returns whatever it collected within the timeout; with the gate off it must
	// stay empty.
	samples, _ := deps.Demultiplexer.WaitForNumberOfSamples(1, 0, 500*time.Millisecond)
	assert.Empty(t, samples, "no samples should reach the sampler when dogstatsd_flush_incomplete_buckets is unset")
}
