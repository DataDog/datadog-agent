// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package probe

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
)

func TestScanReturnsOneResultPerTargetInOrder(t *testing.T) {
	targets := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}

	results, err := scan(context.Background(), 2, targets, func(_ context.Context, target string) Result {
		return Result{Target: target}
	})

	require.NoError(t, err)
	require.Len(t, results, 3)
	for i, target := range targets {
		assert.Equal(t, target, results[i].Target)
	}
}

func TestScanKeepsEveryOtherResultWhenOneTargetFails(t *testing.T) {
	targets := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}

	results, err := scan(context.Background(), 3, targets, func(_ context.Context, target string) Result {
		res := Result{Target: target, Ping: &pingprobe.Reading{Success: true}}
		if target == "10.0.0.2" {
			res.Ping = &pingprobe.Reading{FailureReason: "unreachable"}
		}
		return res
	})

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.True(t, results[0].Ping.Success)
	assert.False(t, results[1].Ping.Success)
	assert.True(t, results[2].Ping.Success)
}

// recordPeak tracks the highest number of callbacks in flight at once.
func recordPeak(running, peak *atomic.Int64) int64 {
	current := running.Add(1)
	for {
		observed := peak.Load()
		if current <= observed || peak.CompareAndSwap(observed, current) {
			break
		}
	}
	return current
}

func TestScanNeverRunsMoreTargetsThanWorkers(t *testing.T) {
	const workers = 2
	var running, peak, started atomic.Int64
	// Holds the first batch until the whole budget is in flight, so the peak is
	// reached without waiting on the scheduler.
	barrier := make(chan struct{})

	_, err := scan(context.Background(), workers, []string{"a", "b", "c", "d", "e", "f"}, func(_ context.Context, target string) Result {
		recordPeak(&running, &peak)
		if started.Add(1) == workers {
			close(barrier)
		}
		<-barrier
		running.Add(-1)
		return Result{Target: target}
	})

	require.NoError(t, err)
	assert.Equal(t, int64(workers), peak.Load(), "the whole budget runs at once, and never more than it")
}

func TestScanClampsANonPositiveWorkerCountToOne(t *testing.T) {
	var running, peak atomic.Int64

	results, err := scan(context.Background(), 0, []string{"a", "b", "c"}, func(_ context.Context, target string) Result {
		recordPeak(&running, &peak)
		// Yields so a second callback would be observed if one were dispatched.
		runtime.Gosched()
		running.Add(-1)
		return Result{Target: target}
	})

	require.NoError(t, err, "a zero worker count must not deadlock on an unbuffered semaphore")
	require.Len(t, results, 3)
	assert.Equal(t, int64(1), peak.Load())
}

func TestScanReturnsTheCollectedResultsWithItsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var done atomic.Int64

	results, err := scan(ctx, 1, []string{"a", "b", "c"}, func(_ context.Context, target string) Result {
		if done.Add(1) == 1 {
			cancel()
		}
		return Result{Target: target}
	})

	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, results, 1, "the second target is never dispatched")
	assert.Equal(t, "a", results[0].Target)
}

func TestScanDispatchesNothingWhenItsContextIsAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int64

	results, err := scan(ctx, 2, []string{"a", "b", "c"}, func(_ context.Context, target string) Result {
		calls.Add(1)
		return Result{Target: target}
	})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, calls.Load())
	assert.Empty(t, results)
}

func TestScanOneRunsOnlyTheConfiguredProbes(t *testing.T) {
	res := scanOne(context.Background(), "127.0.0.1", Options{
		SNMP: &snmpprobe.Options{
			Port:        1,
			Timeout:     50 * time.Millisecond,
			Credentials: []snmpprobe.Credential{{ID: "cred-1", Version: "2c", Community: "public"}},
		},
	})

	assert.Equal(t, "127.0.0.1", res.Target)
	assert.Nil(t, res.Ping)
	require.NotNil(t, res.SNMP)
	assert.False(t, res.SNMP.Success)
}

func TestScanOneRunsNoProbeForEmptyOptions(t *testing.T) {
	res := scanOne(context.Background(), "10.0.0.1", Options{})

	assert.Equal(t, "10.0.0.1", res.Target)
	assert.Nil(t, res.Ping)
	assert.Nil(t, res.SNMP)
}

func TestEmptyReportsWhetherAProbeIsConfigured(t *testing.T) {
	assert.True(t, Options{}.Empty())
	assert.False(t, Options{Ping: &pingprobe.Options{}}.Empty())
	assert.False(t, Options{SNMP: &snmpprobe.Options{}}.Empty())
}

func TestFingerprintsHoldOneEntryPerConfiguredProbeInProbeOrder(t *testing.T) {
	opts := Options{
		Ping: &pingprobe.Options{Count: 1},
		SNMP: &snmpprobe.Options{Port: 161},
	}

	fps := opts.Fingerprints()

	require.Len(t, fps, 2)
	assert.Equal(t, opts.Ping.Fingerprint(), fps[0])
	assert.Equal(t, opts.SNMP.Fingerprint(), fps[1])
	assert.Empty(t, Options{}.Fingerprints())
}

func TestScanWithNoProbeStillReturnsATargetPerAddress(t *testing.T) {
	results, err := Scan(context.Background(), 2, []string{"10.0.0.1", "10.0.0.2"}, Options{})

	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "10.0.0.1", results[0].Target)
	assert.Equal(t, "10.0.0.2", results[1].Target)
}
