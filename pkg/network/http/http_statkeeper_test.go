// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package http

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHTTPTransactions(t *testing.T) {
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 1000
	tel, err := newTelemetry()
	require.NoError(t, err)
	sk := newHTTPStatkeeper(cfg, tel)
	txs := make([]transaction.HttpTX, 100)

	srcString := "1.1.1.1"
	dstString := "2.2.2.2"
	sourceIP := util.AddressFromString(srcString)
	sourcePort := 1234
	destIP := util.AddressFromString(dstString)
	destPort := 8080

	const numPaths = 10
	for i := 0; i < numPaths; i++ {
		path := "/testpath" + strconv.Itoa(i)

		for j := 0; j < 10; j++ {
			statusCode := (j%5 + 1) * 100
			latency := time.Duration(j%5+1) * time.Millisecond
			txs[i*10+j] = generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, path, statusCode, latency)
		}
	}

	sk.Process(txs)

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, 0, len(sk.stats))
	assert.Equal(t, numPaths, len(stats))
	for key, stats := range stats {
		assert.Equal(t, "/testpath", key.Path.Content[:9])
		for i := 0; i < 5; i++ {
			s := stats.Stats((i + 1) * 100)
			require.NotNil(t, s)
			assert.Equal(t, 2, s.Count)
			assert.Equal(t, 2.0, s.Latencies.GetCount())

			p50, err := s.Latencies.GetValueAtQuantile(0.5)
			assert.Nil(t, err)

			expectedLatency := float64(time.Duration(i+1) * time.Millisecond)
			acceptableError := expectedLatency * s.Latencies.IndexMapping.RelativeAccuracy()
			assert.True(t, p50 >= expectedLatency-acceptableError)
			assert.True(t, p50 <= expectedLatency+acceptableError)
		}
	}
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
	transactions := []transaction.HttpTX{tx}

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
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 1000
	setupStatKeeper := func(rules []*config.ReplaceRule) *httpStatKeeper {
		c := cfg
		c.HTTPReplaceRules = rules

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
		transactions := []transaction.HttpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/foobar", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/payment/123", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key := range stats {
			assert.Equal(t, "/foobar", key.Path.Content)
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
		transactions := []transaction.HttpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/1", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/2", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/3", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/prefix/users/?", key.Path.Content)
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
		transactions := []transaction.HttpTX{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/ana/payment/123", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/bob/payment/456", statusCode, latency),
		}
		sk.Process(transactions)
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/users/?/payment/?", key.Path.Content)
			s := metrics.Stats(statusCode)
			require.NotNil(t, s)
			assert.Equal(t, 2, s.Count)
		}
	})
}

func TestHTTPCorrectness(t *testing.T) {
	t.Run("wrong path format", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel, err := newTelemetry()
		require.NoError(t, err)
		sk := newHTTPStatkeeper(cfg, tel)
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/ver\x04y/wro\x02g/path/",
			404,
			30*time.Millisecond,
		)
		transactions := []transaction.HttpTX{tx}

		sk.Process(transactions)
		tel.log()
		require.Equal(t, int64(1), tel.malformed.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("invalid http verb", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel, err := newTelemetry()
		require.NoError(t, err)
		sk := newHTTPStatkeeper(cfg, tel)
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/ver\x04y/wro\x02g/path/",
			404,
			30*time.Millisecond,
		)
		tx.SetRequestMethod(0) /* This is MethodUnknown */
		transactions := []transaction.HttpTX{tx}

		sk.Process(transactions)
		tel.log()
		require.Equal(t, int64(1), tel.malformed.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("invalid latency", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel, err := newTelemetry()
		require.NoError(t, err)
		sk := newHTTPStatkeeper(cfg, tel)
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/ver\x04y/wro\x02g/path/",
			404,
			0,
		)
		transactions := []transaction.HttpTX{tx}

		sk.Process(transactions)
		tel.log()
		require.Equal(t, int64(1), tel.malformed.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})
}
