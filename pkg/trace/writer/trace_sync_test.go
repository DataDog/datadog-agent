// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package writer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestTraceSyncWriter(t *testing.T) {
	// TestTraceWriter
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledSpans{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		in := make(chan *SampledSpans)
		tw := NewTraceSyncWriter(cfg, in)
		go tw.Run()
		for _, ss := range testSpans {
			in <- ss
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.SyncFlush()
		// Now all trace payloads should be sent
		assert.Equal(t, 3, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans)
	})
}

func TestTraceSyncWriterStop(t *testing.T) {
	// TestTraceWriter
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledSpans{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		in := make(chan *SampledSpans)
		tw := NewTraceSyncWriter(cfg, in)
		go tw.Run()
		for _, ss := range testSpans {
			in <- ss
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.Stop()
		// Now all trace payloads should be sent
		assert.Equal(t, 3, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans)
	})
}
