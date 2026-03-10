// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// makeForwarder returns a DefaultForwarder connected to the given domain/key map.
// It does NOT call Start() — use this for benchmarks that exercise only
// createHTTPTransactions or other non-network paths.
func makeForwarder(b *testing.B, domains map[string][]configutils.APIKeys) *DefaultForwarder {
	b.Helper()
	mockConfig := configmock.New(b)
	log := logmock.New(b)
	secrets := secretsmock.New(b)
	r, err := resolver.NewSingleDomainResolvers(domains)
	if err != nil {
		b.Fatalf("NewSingleDomainResolvers: %v", err)
	}
	opts := NewOptionsWithResolvers(mockConfig, log, r)
	opts.Secrets = secrets
	return NewDefaultForwarder(mockConfig, log, opts)
}

// makeStartedForwarder creates and starts a forwarder pointed at the given test server URL.
func makeStartedForwarder(b *testing.B, serverURL string) *DefaultForwarder {
	b.Helper()
	domains := map[string][]configutils.APIKeys{
		serverURL: {configutils.NewAPIKeys("path", "bench-api-key")},
	}
	f := makeForwarder(b, domains)
	if err := f.Start(); err != nil {
		b.Fatalf("Start: %v", err)
	}
	b.Cleanup(func() { f.Stop() })
	return f
}

// reportForwarderPercentiles emits p50/p95/p99 tail-latency metrics.
func reportForwarderPercentiles(b *testing.B, durations []int64) {
	b.Helper()
	n := len(durations)
	if n == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[n/2]
	p95idx := int(float64(n)*0.95) - 1
	if p95idx < 0 {
		p95idx = 0
	}
	p99idx := int(float64(n)*0.99) - 1
	if p99idx < 0 {
		p99idx = 0
	}
	b.ReportMetric(float64(p50), "p50-ns")
	b.ReportMetric(float64(durations[p95idx]), "p95-ns")
	b.ReportMetric(float64(durations[p99idx]), "p99-ns")
	if p50 > 0 {
		b.ReportMetric(float64(durations[p99idx])/float64(p50), "p99/p50-ratio")
	}
}

// ---------------------------------------------------------------------------
// Transaction creation benchmarks
// createHTTPTransactions() is called once per flush event to build the set of
// HTTPTransaction objects that workers will submit. It iterates over all API
// keys and domain resolvers, so cost scales with key and domain count.
// ---------------------------------------------------------------------------

// BenchmarkCreateHTTPTransactionPayloadSize measures how payload size affects
// transaction creation cost (primarily header allocation and resolver iteration).
func BenchmarkCreateHTTPTransactionPayloadSize(b *testing.B) {
	const domain = "http://app.datadoghq.com"
	f := makeForwarder(b, map[string][]configutils.APIKeys{
		domain: {configutils.NewAPIKeys("path", "api-key-1")},
	})

	for _, payloadBytes := range []int{256, 4096, 65536, 1 << 20} {
		payloadBytes := payloadBytes
		b.Run(fmt.Sprintf("%d-bytes", payloadBytes), func(b *testing.B) {
			payload := make([]byte, payloadBytes)
			payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload})
			headers := make(http.Header)

			durations := make([]int64, b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
				durations[i] = time.Since(start).Nanoseconds()
				_ = txns
			}
			b.StopTimer()
			b.ReportAllocs()
			b.ReportMetric(float64(payloadBytes), "payload-bytes")
			reportForwarderPercentiles(b, durations)
		})
	}
}

// BenchmarkCreateHTTPTransactionMultiPayload measures the cost of building
// transactions for multiple payloads in a single flush (e.g. after payload splitting).
func BenchmarkCreateHTTPTransactionMultiPayload(b *testing.B) {
	const domain = "http://app.datadoghq.com"
	f := makeForwarder(b, map[string][]configutils.APIKeys{
		domain: {configutils.NewAPIKeys("path", "api-key-1")},
	})

	for _, numPayloads := range []int{1, 4, 16, 64} {
		numPayloads := numPayloads
		b.Run(fmt.Sprintf("%d-payloads", numPayloads), func(b *testing.B) {
			chunks := make([]*[]byte, numPayloads)
			for i := range chunks {
				p := make([]byte, 4096)
				chunks[i] = &p
			}
			payloads := transaction.NewBytesPayloadsWithoutMetaData(chunks)
			headers := make(http.Header)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
				_ = txns
			}
		})
	}
}

// BenchmarkCreateHTTPTransactionMultiAPIKey measures how API key count affects
// transaction creation. More keys → more transactions per flush.
func BenchmarkCreateHTTPTransactionMultiAPIKey(b *testing.B) {
	for _, numKeys := range []int{1, 2, 4, 8} {
		numKeys := numKeys
		b.Run(fmt.Sprintf("%d-api-keys", numKeys), func(b *testing.B) {
			keys := make([]string, numKeys)
			for i := range keys {
				keys[i] = fmt.Sprintf("api-key-%d", i)
			}
			f := makeForwarder(b, map[string][]configutils.APIKeys{
				"http://app.datadoghq.com": {configutils.NewAPIKeys("path", keys...)},
			})

			payload := make([]byte, 4096)
			payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload})
			headers := make(http.Header)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
				_ = txns
			}
		})
	}
}

// BenchmarkCreateHTTPTransactionMultiDomain measures transaction creation cost
// when payloads are replicated to multiple backend domains (e.g. MRF, shadow).
func BenchmarkCreateHTTPTransactionMultiDomain(b *testing.B) {
	for _, numDomains := range []int{1, 2, 3} {
		numDomains := numDomains
		b.Run(fmt.Sprintf("%d-domains", numDomains), func(b *testing.B) {
			domains := make(map[string][]configutils.APIKeys, numDomains)
			for i := 0; i < numDomains; i++ {
				domain := fmt.Sprintf("http://datadoghq-%d.com", i)
				domains[domain] = []configutils.APIKeys{
					configutils.NewAPIKeys("path", fmt.Sprintf("api-key-%d", i)),
				}
			}
			f := makeForwarder(b, domains)

			payload := make([]byte, 4096)
			payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload})
			headers := make(http.Header)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
				_ = txns
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Transaction submission benchmarks
// sendHTTPTransactions() enqueues transactions into the domain forwarder
// priority queues. These benchmarks measure queue submission overhead,
// not network I/O (that is covered by end-to-end tests).
// ---------------------------------------------------------------------------

// BenchmarkSendHTTPTransactionsLowThroughput measures queue submission for
// a single small payload — baseline for the submission path.
func BenchmarkSendHTTPTransactionsLowThroughput(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	b.Cleanup(ts.Close)

	f := makeStartedForwarder(b, ts.URL)

	payload := make([]byte, 256)
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload})
	headers := make(http.Header)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
		if err := f.sendHTTPTransactions(txns); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSendHTTPTransactionsHighThroughput measures queue submission with
// large batch payloads — exercises priority queue throughput under load.
func BenchmarkSendHTTPTransactionsHighThroughput(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	b.Cleanup(ts.Close)

	f := makeStartedForwarder(b, ts.URL)

	// 16 payloads of 64KB each — representative of a large flush
	chunks := make([]*[]byte, 16)
	for i := range chunks {
		p := make([]byte, 65536)
		chunks[i] = &p
	}
	payloads := transaction.NewBytesPayloadsWithoutMetaData(chunks)
	headers := make(http.Header)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
		if err := f.sendHTTPTransactions(txns); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSendHTTPTransactionsPriorityMix tests the queue under mixed priority —
// simulates a flush that includes both regular series (normal) and
// host metadata (high priority).
func BenchmarkSendHTTPTransactionsPriorityMix(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	b.Cleanup(ts.Close)

	f := makeStartedForwarder(b, ts.URL)

	payload := make([]byte, 4096)
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload})
	headers := make(http.Header)

	durations := make([]int64, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		// Alternate between normal (Series) and high-priority (HostMetadata) submissions
		if i%2 == 0 {
			txns := f.createHTTPTransactions(endpoints.SeriesEndpoint, payloads, transaction.Series, headers)
			if err := f.sendHTTPTransactions(txns); err != nil {
				b.Fatal(err)
			}
		} else {
			txns := f.createHTTPTransactions(endpoints.HostMetadataEndpoint, payloads, transaction.Metadata, headers)
			if err := f.sendHTTPTransactions(txns); err != nil {
				b.Fatal(err)
			}
		}
		durations[i] = time.Since(start).Nanoseconds()
	}
	b.StopTimer()
	b.ReportAllocs()
	reportForwarderPercentiles(b, durations)
}
