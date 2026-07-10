// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

const (
	discoverySuccessBucket = uint16(200)
	discoveryErrorBucket   = uint16(400)
	discoveryTestSport     = uint16(1234)
	discoveryTestDport     = uint16(8080)
)

// newDiscoveryStatkeeper builds the shared http.StatKeeper in discovery mode,
// exactly as the HTTP/2 protocol wires it in protocol.go.
func newDiscoveryStatkeeper(t *testing.T) *http.StatKeeper {
	t.Helper()
	cfg := config.New()
	cfg.DiscoveryServiceMapEnabled = true
	tel := http.NewTelemetry("http2")
	return http.NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg))
}

// makeHTTP2Tx builds a complete HTTP/2 transaction on a fixed 4-tuple.
func makeHTTP2Tx(status uint16, latency time.Duration) *EventWrapper {
	ew := &EventWrapper{EbpfTx: &EbpfTx{
		Tuple: ConnTuple{Saddr_l: 1, Daddr_l: 2, Sport: discoveryTestSport, Dport: discoveryTestDport},
		Stream: HTTP2Stream{
			Request_started:    1,
			Response_last_seen: 1 + uint64(latency.Nanoseconds()),
			Path:               http2Path{Finalized: true},
		},
	}}
	ew.SetStatusCode(status)
	ew.SetRequestMethod(http.MethodGet)
	return ew
}

func processHTTP2(sk *http.StatKeeper, statuses []int, latency time.Duration) (successes, errors int) {
	for _, code := range statuses {
		sk.Process(makeHTTP2Tx(uint16(code), latency))
		if code >= 400 {
			errors++
		} else {
			successes++
		}
	}
	return successes, errors
}

func TestDiscoveryMode_HTTP2CollapsesIntoSingleEntry(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)

	statuses := []int{200, 201, 301, 302, 400, 404, 500, 502, 503}
	wantSuccesses, wantErrors := processHTTP2(sk, statuses, time.Millisecond)

	stats := sk.GetAndResetAllStats()
	require.Len(t, stats, 1, "all HTTP/2 streams on one connection collapse into one entry")

	for key, rs := range stats {
		assert.Empty(t, key.Path.Content.Get(), "path dropped in discovery mode")
		assert.Equal(t, http.MethodUnknown, key.Method, "method dropped in discovery mode")

		require.Contains(t, rs.Data, discoverySuccessBucket)
		require.Contains(t, rs.Data, discoveryErrorBucket)
		assert.Len(t, rs.Data, 2, "only success + error buckets")
		assert.Equal(t, wantSuccesses, rs.Data[discoverySuccessBucket].Count)
		assert.Equal(t, wantErrors, rs.Data[discoveryErrorBucket].Count)
	}
}

func TestDiscoveryMode_HTTP2NoDDSketch(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)
	processHTTP2(sk, []int{200, 500}, time.Millisecond)

	for _, rs := range sk.GetAndResetAllStats() {
		for bucket, stat := range rs.Data {
			assert.Nil(t, stat.Latencies, "bucket %d: no DDSketch in discovery mode", bucket)
		}
	}
}

func TestDiscoveryMode_HTTP2LatencySumAccumulates(t *testing.T) {
	sk := newDiscoveryStatkeeper(t)

	perTx := makeHTTP2Tx(200, time.Millisecond).RequestLatency()
	require.Positive(t, perTx)

	const numRequests = 5
	statuses := make([]int, numRequests)
	for i := range statuses {
		statuses[i] = 200
	}
	processHTTP2(sk, statuses, time.Millisecond)

	stats := sk.GetAndResetAllStats()
	require.Len(t, stats, 1)
	for _, rs := range stats {
		bucket := rs.Data[discoverySuccessBucket]
		require.NotNil(t, bucket)
		assert.Equal(t, numRequests, bucket.Count)
		assert.InDelta(t, perTx*float64(numRequests), bucket.LatencySum, 1.0)
		assert.Zero(t, bucket.FirstLatencySample, "discovery mode uses LatencySum, not FirstLatencySample")
		assert.Nil(t, bucket.Latencies)
	}
}

func TestDiscoveryMode_HTTP2RejectsInvalidTransactions(t *testing.T) {
	tests := []struct {
		name    string
		status  uint16
		latency time.Duration
	}{
		{"zero latency", 200, 0},
		{"status below valid range", 99, time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sk := newDiscoveryStatkeeper(t)
			sk.Process(makeHTTP2Tx(tc.status, tc.latency))
			assert.Empty(t, sk.GetAndResetAllStats(), "invalid transaction should be dropped")
		})
	}
}
