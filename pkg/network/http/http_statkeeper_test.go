// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHTTPTransactions(t *testing.T) {
	cfg := &config.Config{MaxHTTPStatsBuffered: 1000}
	tel, err := newTelemetry()
	require.NoError(t, err)
	sk := newHTTPStatkeeper(cfg, tel)
	txs := make([]httpTX, 100)

	sourceIP := util.AddressFromString("1.1.1.1")
	sourcePort := 1234
	destIP := util.AddressFromString("2.2.2.2")
	destPort := 8080

	const numPaths = 10
	for i := 0; i < numPaths; i++ {
		path := "/testpath" + strconv.Itoa(i)

		for j := 0; j < 10; j++ {
			statusCode := (j%5 + 1) * 100
			latency := time.Duration(j%5) * time.Millisecond
			txs[i*10+j] = generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, path, statusCode, latency)
		}
	}

	sk.Process(txs)

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, 0, len(sk.stats))
	assert.Equal(t, numPaths, len(stats))
	for key, stats := range stats {
		assert.Equal(t, "/testpath", key.Path[:9])
		for i := 0; i < 5; i++ {
			s := stats.Stats((i + 1) * 100)
			require.NotNil(t, s)
			assert.Equal(t, 2, s.Count)
			assert.Equal(t, 2.0, s.Latencies.GetCount())

			p50, err := s.Latencies.GetValueAtQuantile(0.5)
			assert.Nil(t, err)

			expectedLatency := float64(time.Duration(i) * time.Millisecond)
			acceptableError := expectedLatency * s.Latencies.IndexMapping.RelativeAccuracy()
			assert.True(t, p50 >= expectedLatency-acceptableError)
			assert.True(t, p50 <= expectedLatency+acceptableError)
		}
	}
}

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) httpTX {
	var tx httpTX

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := _Ctype_ulonglong(uint64(latency))
	tx.request_started = 1
	tx.response_last_seen = tx.request_started + latencyNS
	tx.response_status_code = _Ctype_ushort(code)
	tx.request_fragment = requestFragment([]byte(reqFragment))
	tx.tup.saddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(source.Bytes()))
	tx.tup.sport = _Ctype_ushort(sourcePort)
	tx.tup.daddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(dest.Bytes()))
	tx.tup.dport = _Ctype_ushort(destPort)
	tx.tup.metadata = 1

	return tx
}

func BenchmarkProcessSameConn(b *testing.B) {
	cfg := &config.Config{MaxHTTPStatsBuffered: 1000}
	tel, err := newTelemetry()
	require.NoError(b, err)
	sk := newHTTPStatkeeper(cfg, tel)
	tx := generateIPv4HTTPTransaction(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		1234,
		8080,
		"foobar",
		404,
		30*time.Millisecond,
	)
	transactions := []httpTX{tx}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.Process(transactions)
	}
}

func TestPathProcessing(t *testing.T) {
	var (
		sourceIP   = util.AddressFromString("1.1.1.1")
		sourcePort = 1234
		destIP     = util.AddressFromString("2.2.2.2")
		destPort   = 8080
		statusCode = 200
		latency    = time.Second
	)

	setupStatKeeper := func(rules []*config.ReplaceRule) *httpStatKeeper {
		c := &config.Config{
			MaxHTTPStatsBuffered: 1000,
			HTTPReplaceRules:     rules,
		}

		tel, err := newTelemetry()
		require.NoError(t, err)
		return newHTTPStatkeeper(c, tel)
	}

	t.Run("reject rule", func(t *testing.T) {
		rules := []*config.ReplaceRule{
			{
				Re: regexp.MustCompile("payment"),
			},
		}

		sk := setupStatKeeper(rules)
		transactions := []httpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/foobar", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/payment/123", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key := range stats {
			assert.Equal(t, "/foobar", key.Path)
		}
	})

	t.Run("replace rule", func(t *testing.T) {
		rules := []*config.ReplaceRule{
			{
				Re:   regexp.MustCompile("/users/.*"),
				Repl: "/users/?",
			},
		}

		sk := setupStatKeeper(rules)
		transactions := []httpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/1", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/2", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/3", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/prefix/users/?", key.Path)
			s := metrics.Stats(statusCode)
			require.NotNil(t, s)
			assert.Equal(t, 3, s.Count)
		}
	})

	t.Run("chained rules", func(t *testing.T) {
		rules := []*config.ReplaceRule{
			{
				Re:   regexp.MustCompile("/users/[A-z0-9]+"),
				Repl: "/users/?",
			},
			{
				Re:   regexp.MustCompile("/payment/[0-9]+"),
				Repl: "/payment/?",
			},
		}

		sk := setupStatKeeper(rules)
		transactions := []httpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/ana/payment/123", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/bob/payment/456", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/users/?/payment/?", key.Path)
			s := metrics.Stats(statusCode)
			require.NotNil(t, s)
			assert.Equal(t, 2, s.Count)
		}
	})

}
