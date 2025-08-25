// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHTTPTransactions(t *testing.T) {
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 1000
	tel := NewTelemetry("http")
	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))

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
			tx := generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, path, statusCode, latency)
			sk.Process(tx)
		}
	}

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, 0, len(sk.stats))
	assert.Equal(t, numPaths, len(stats))
	for key, stats := range stats {
		assert.Equal(t, "/testpath", key.Path.Content.Get()[:9])
		for i := 0; i < 5; i++ {
			s := stats.Data[uint16((i+1)*100)]
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

func BenchmarkProcessHTTPTransactions(b *testing.B) {
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 100000
	tel := NewTelemetry("http")
	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))

	srcString := "1.1.1.1"
	dstString := "2.2.2.2"
	sourceIP := util.AddressFromString(srcString)
	sourcePort := 1234
	destIP := util.AddressFromString(dstString)
	destPort := 8080

	const numPaths = 10000
	const uniqPaths = 50
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for p := 0; p < numPaths; p++ {
			b.StopTimer()
			//we use subset of unique endpoints, but those will occur over and over again like in regular target application
			path := "/testpath/blablabla/dsadas/isdaasd/asdasadsadasd" + strconv.Itoa(p%uniqPaths)
			//we simulate different conn tuples by increasing the port number
			newSourcePort := sourcePort + (p % 30)
			statusCode := (i%5 + 1) * 100
			latency := time.Duration(i%5+1) * time.Millisecond
			tx := generateIPv4HTTPTransaction(sourceIP, destIP, newSourcePort, destPort, path, statusCode, latency)
			b.StartTimer()
			sk.Process(tx)
		}
	}
	b.StopTimer()
}

func BenchmarkProcessSameConn(b *testing.B) {
	cfg := &config.Config{MaxHTTPStatsBuffered: 1000}
	tel := NewTelemetry("http")
	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
	tx := generateIPv4HTTPTransaction(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		1234,
		8080,
		"foobar",
		404,
		30*time.Millisecond,
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sk.Process(tx)
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
	setupStatKeeper := func(rules []*config.ReplaceRule) *StatKeeper {
		c := cfg
		c.HTTPReplaceRules = rules

		tel := NewTelemetry("http")
		return NewStatkeeper(c, tel, NewIncompleteBuffer(cfg, tel))
	}

	t.Run("reject rule", func(t *testing.T) {
		rules := []*config.ReplaceRule{
			{
				Re: regexp.MustCompile("payment"),
			},
		}

		sk := setupStatKeeper(rules)
		transactions := []Transaction{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/foobar", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/payment/123", statusCode, latency),
		}
		for _, tx := range transactions {
			sk.Process(tx)
		}
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key := range stats {
			assert.Equal(t, "/foobar", key.Path.Content.Get())
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
		transactions := []Transaction{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/1", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/2", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/prefix/users/3", statusCode, latency),
		}
		for _, tx := range transactions {
			sk.Process(tx)
		}
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/prefix/users/?", key.Path.Content.Get())
			s := metrics.Data[uint16(statusCode)]
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
		transactions := []Transaction{
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/ana/payment/123", statusCode, latency),
			generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, "/users/bob/payment/456", statusCode, latency),
		}
		for _, tx := range transactions {
			sk.Process(tx)
		}
		stats := sk.GetAndResetAllStats()

		require.Len(t, stats, 1)
		for key, metrics := range stats {
			assert.Equal(t, "/users/?/payment/?", key.Path.Content.Get())
			s := metrics.Data[uint16(statusCode)]
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
		tel := NewTelemetry("http")
		sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/ver\x04y/wro\x02g/path/",
			404,
			30*time.Millisecond,
		)

		sk.Process(tx)
		tel.Log()
		require.Equal(t, int64(1), tel.nonPrintableCharacters.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("invalid http verb", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel := NewTelemetry("http")
		sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/get",
			404,
			30*time.Millisecond,
		)
		tx.SetRequestMethod(MethodUnknown)

		sk.Process(tx)
		tel.Log()
		require.Equal(t, int64(1), tel.unknownMethod.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("invalid latency", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel := NewTelemetry("http")
		sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/get",
			404,
			0,
		)

		sk.Process(tx)
		tel.Log()
		require.Equal(t, int64(1), tel.invalidLatency.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("invalid status code", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel := NewTelemetry("http")
		sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"/get",
			700,
			30*time.Millisecond,
		)

		sk.Process(tx)
		tel.Log()
		require.Equal(t, int64(1), tel.invalidStatusCode.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})

	t.Run("Empty path", func(t *testing.T) {
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = 1000
		libtelemetry.Clear()
		tel := NewTelemetry("http")
		sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
		tx := generateIPv4HTTPTransaction(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			1234,
			8080,
			"",
			404,
			30*time.Millisecond,
		)

		sk.Process(tx)
		tel.Log()
		require.Equal(t, int64(1), tel.emptyPath.Get())

		stats := sk.GetAndResetAllStats()
		require.Len(t, stats, 0)
	})
}

func TestStatkeeperSortedRules(t *testing.T) {
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 1000
	cfg.HTTPReplaceRules = []*config.ReplaceRule{
		{
			Re:   regexp.MustCompile("/users/[A-z0-9]+"),
			Repl: "/users/?",
		},
		{
			Re: regexp.MustCompile("/payment/[0-9]+"),
		},
		{
			Re: regexp.MustCompile("/test"),
		},
		{
			Re:   regexp.MustCompile("/payment2/[0-9]+"),
			Repl: "/payment/?",
		},
	}
	libtelemetry.Clear()
	tel := NewTelemetry("http")

	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))
	require.NotNil(t, sk)

	require.Empty(t, sk.replaceRules[0].Repl)
	require.Equal(t, sk.replaceRules[0].Re.String(), "/payment/[0-9]+")
	require.Empty(t, sk.replaceRules[1].Repl)
	require.Equal(t, sk.replaceRules[1].Re.String(), "/test")
	require.NotEmpty(t, sk.replaceRules[2].Repl)
	require.Equal(t, sk.replaceRules[2].Re.String(), "/users/[A-z0-9]+")
	require.NotEmpty(t, sk.replaceRules[3].Repl)
	require.Equal(t, sk.replaceRules[3].Re.String(), "/payment2/[0-9]+")
}
