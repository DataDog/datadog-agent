// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	testClientIP   = "1.1.1.1"
	testServerIP   = "2.2.2.2"
	testClientPort = 1234
	testServerPort = 8080

	// Status code bucket keys used by discovery mode.
	successBucket = uint16(200)
	errorBucket   = uint16(500)

	statusCodeBelowValidRange = 99
)

// newDiscoveryStatkeeper builds a StatKeeper configured for discovery mode.
func newDiscoveryStatkeeper(t *testing.T) *StatKeeper {
	t.Helper()
	cfg := config.New()
	cfg.DiscoveryServiceMapEnabled = true
	tel := NewTelemetry("http")
	return NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
}

// processTransactions generates one transaction per status code on the same
// 4-tuple but varying paths, and returns how many successes and errors were
// processed (status >= 400 counts as error).
func processTransactions(sk *StatKeeper, statusCodes []int) (successes, errors int) {
	srcIP := util.AddressFromString(testClientIP)
	dstIP := util.AddressFromString(testServerIP)
	for i, code := range statusCodes {
		path := "/path-" + strconv.Itoa(i)
		tx := generateIPv4HTTPTransaction(srcIP, dstIP, testClientPort, testServerPort, path, code, time.Millisecond)
		sk.Process(tx)
		if code >= 400 {
			errors++
		} else {
			successes++
		}
	}
	return successes, errors
}

func TestDiscoveryMode_StatkeeperConfig(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)

	assert.True(t, sk.discoveryMode, "discovery mode flag should be set")
	assert.Equal(t, discoveryMaxStatsBuffered, sk.maxEntries, "max entries should match discovery constant")
}

func TestDiscoveryMode_CollapsesPathsIntoSingleEntry(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)

	// Repeated status codes across many paths — all should collapse into a
	// single connection-key entry with two status buckets.
	statusCodes := []int{200, 201, 301, 302, 400, 404, 500, 502, 503}
	wantSuccesses, wantErrors := processTransactions(sk, statusCodes)

	stats := sk.GetAndResetAllStats()
	require.Len(t, stats, 1, "all paths should collapse into one entry")

	for key, rs := range stats {
		assert.Empty(t, key.Path.Content.Get(), "path should be empty in discovery mode")
		assert.Equal(t, MethodUnknown, key.Method, "method should be unknown in discovery mode")

		require.Contains(t, rs.Data, successBucket)
		require.Contains(t, rs.Data, errorBucket)
		assert.Len(t, rs.Data, 2, "expected only success + error buckets")

		assert.Equal(t, wantSuccesses, rs.Data[successBucket].Count, "success count")
		assert.Equal(t, wantErrors, rs.Data[errorBucket].Count, "error count")
	}
}

func TestDiscoveryMode_NoLatencyTracking(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)

	_, _ = processTransactions(sk, []int{200, 500})

	for _, rs := range sk.GetAndResetAllStats() {
		for bucket, stat := range rs.Data {
			assert.Nil(t, stat.Latencies, "bucket %d: no DDSketch in discovery mode", bucket)
			assert.Zero(t, stat.FirstLatencySample, "bucket %d: no latency sample", bucket)
		}
	}
}

func TestDiscoveryMode_RejectsInvalidTransactions(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		latency    time.Duration
	}{
		{"zero latency", 200, 0},
		{"invalid status code", statusCodeBelowValidRange, time.Millisecond},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sk := newDiscoveryStatkeeper(t)
			srcIP := util.AddressFromString(testClientIP)
			dstIP := util.AddressFromString(testServerIP)

			tx := generateIPv4HTTPTransaction(srcIP, dstIP, testClientPort, testServerPort, "/foo", tc.statusCode, tc.latency)
			sk.Process(tx)

			assert.Empty(t, sk.stats, "invalid transaction should be dropped")
		})
	}
}
