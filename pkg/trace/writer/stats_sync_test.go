// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package writer

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/stretchr/testify/assert"
)

func TestStatsSyncWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		sw, statsChannel, srv := testStatsSyncWriter()
		go sw.Run()

		testSets := [][]stats.Bucket{
			{
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
			},
			{
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
			},
		}

		statsChannel <- testSets[0]
		statsChannel <- testSets[1]

		sw.SyncFlush()

		expectedHeaders := map[string]string{
			"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
			"Content-Type":                 "application/json",
			"Content-Encoding":             "gzip",
			"Dd-Api-Key":                   "123",
		}
		assertPayload(assert, expectedHeaders, testSets, srv.Payloads())
	})
	t.Run("stop", func(t *testing.T) {
		assert := assert.New(t)
		sw, statsChannel, srv := testStatsSyncWriter()
		go sw.Run()

		testSets := [][]stats.Bucket{
			{
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
			},
			{
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
				testutil.RandomBucket(3),
			},
		}

		statsChannel <- testSets[0]
		statsChannel <- testSets[1]

		sw.Stop()

		expectedHeaders := map[string]string{
			"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
			"Content-Type":                 "application/json",
			"Content-Encoding":             "gzip",
			"Dd-Api-Key":                   "123",
		}
		assertPayload(assert, expectedHeaders, testSets, srv.Payloads())
	})
}

func testStatsSyncWriter() (*StatsSyncWriter, chan []stats.Bucket, *testServer) {
	srv := newTestServer()
	// We use a blocking channel to make sure that sends get received on the
	// other end.
	in := make(chan []stats.Bucket)
	cfg := &config.AgentConfig{
		Hostname:    testHostname,
		DefaultEnv:  testEnv,
		Endpoints:   []*config.Endpoint{{Host: srv.URL, APIKey: "123"}},
		StatsWriter: &config.WriterConfig{ConnectionLimit: 20, QueueSize: 20},
	}
	return NewStatsSyncWriter(cfg, in), in, srv
}
