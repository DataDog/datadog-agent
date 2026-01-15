// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

func TestServerlessBatcherServiceCheckFlush(t *testing.T) {
	cfg := mock.New(t)
	deps := fulfillDeps(t)
	// our logger will log dogstatsd packet by default if nothing is setup
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)

	histogram := deps.Telemetry.NewHistogram("test-dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		defaultChannelBuckets)

	demux := deps.Demultiplexer
	batcher := newServerlessBatcher(demux, histogram)

	assert.Nil(t, batcher.choutServiceChecks, "Expected serverless batcher to have nil service checks channel")

	batcher.appendServiceCheck(&servicecheck.ServiceCheck{
		CheckName: "agent.up",
		Host:      "localhost",
		Message:   "this is fine",
		Tags:      []string{"sometag1:somevalyyue1", "sometag2:somevalue2"},
		Status:    0,
		Ts:        12345,
	})

	assert.Len(t, batcher.serviceChecks, 1, "Expected serviceChecks to have 1 service check before flush")

	done := make(chan struct{})
	go func() {
		batcher.flush()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout after waiting 1s for service check flush") // timeout can occur if there is an attempt to flush to a nil channel
	}

	assert.Len(t, batcher.serviceChecks, 0, "Expected serviceChecks to be emptied after flush")
}

func TestServerlessBatcherEventFlush(t *testing.T) {
	cfg := mock.New(t)
	deps := fulfillDeps(t)
	// our logger will log dogstatsd packet by default if nothing is setup
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)

	histogram := deps.Telemetry.NewHistogram("test-dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		defaultChannelBuckets)

	demux := deps.Demultiplexer
	batcher := newServerlessBatcher(demux, histogram)

	assert.Nil(t, batcher.choutEvents, "Expected serverless batcher to have nil events channel")

	batcher.appendEvent(&event.Event{
		Title:     "test title",
		Text:      "test\ntext",
		Tags:      []string{"tag1", "tag2:test"},
		Host:      "some.host",
		Ts:        12345,
		AlertType: event.AlertTypeWarning,

		Priority:       event.PriorityLow,
		AggregationKey: "aggKey",
		SourceTypeName: "source test",
	})

	assert.Len(t, batcher.events, 1, "Expected events to have 1 event before flush")

	done := make(chan struct{})
	go func() {
		batcher.flush()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout after waiting 1s for event flush") // timeout can occur if there is an attempt to flush to a nil channel
	}

	assert.Len(t, batcher.events, 0, "Expected events to be emptied after flush")
}

func TestBatcherServiceCheckFlush(t *testing.T) {
	cfg := mock.New(t)
	deps := fulfillDeps(t)
	// our logger will log dogstatsd packet by default if nothing is setup
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)

	histogram := deps.Telemetry.NewHistogram("test-dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		defaultChannelBuckets)

	demux := deps.Demultiplexer
	_, serviceOut := demux.GetEventsAndServiceChecksChannels()
	batcher := newBatcher(demux, histogram)

	assert.NotNil(t, batcher.choutServiceChecks, "Expected batcher to have service checks channel")

	batcher.appendServiceCheck(&servicecheck.ServiceCheck{
		CheckName: "agent.up",
		Host:      "localhost",
		Message:   "this is fine",
		Tags:      []string{"sometag1:somevalyyue1", "sometag2:somevalue2"},
		Status:    0,
		Ts:        12345,
	})

	assert.Len(t, batcher.serviceChecks, 1, "Expected serviceChecks to have 1 service check before flush")

	done := make(chan struct{})
	go func() {
		batcher.flush()
		close(done)
	}()

	select {
	case <-serviceOut:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout after waiting 1s for service check flush")
	}

	<-done
	assert.Len(t, batcher.serviceChecks, 0, "Expected serviceChecks to be emptied after flush")
}

func TestBatcherEventFlush(t *testing.T) {
	cfg := mock.New(t)
	deps := fulfillDeps(t)
	// our logger will log dogstatsd packet by default if nothing is setup
	pkglogsetup.SetupLogger("", "off", "", "", false, true, false, cfg)

	histogram := deps.Telemetry.NewHistogram("test-dogstatsd",
		"channel_latency",
		[]string{"shard", "message_type"},
		"Time in nanosecond to push metrics to the aggregator input buffer",
		defaultChannelBuckets)

	demux := deps.Demultiplexer
	eventOut, _ := demux.GetEventsAndServiceChecksChannels()
	batcher := newBatcher(demux, histogram)

	assert.NotNil(t, batcher.choutEvents, "Expected batcher to have events channel")

	batcher.appendEvent(&event.Event{
		Title:     "test title",
		Text:      "test\ntext",
		Tags:      []string{"tag1", "tag2:test"},
		Host:      "some.host",
		Ts:        12345,
		AlertType: event.AlertTypeWarning,

		Priority:       event.PriorityLow,
		AggregationKey: "aggKey",
		SourceTypeName: "source test",
	})

	assert.Len(t, batcher.events, 1, "Expected events to have 1 event before flush")

	done := make(chan struct{})
	go func() {
		batcher.flush()
		close(done)
	}()

	select {
	case <-eventOut:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout after waiting 1s for event flush")
	}

	<-done
	assert.Len(t, batcher.events, 0, "Expected events to be emptied after flush")
}
