// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func logMessage(content string) *message.Message {
	origin := message.NewOrigin(sources.NewLogSource("nginx", &config.LogsConfig{Source: "nginx", Service: "web"}))
	m := message.NewMessage([]byte(content), origin, "", 0)
	m.RawDataLen = len(content)
	return m
}

type fakeDestination struct {
	client.Destination
	target string
	mrf    bool
}

func (d fakeDestination) Target() string { return d.target }
func (d fakeDestination) IsMRF() bool    { return d.mrf }

func TestWorkerDropTargets(t *testing.T) {
	reliable := workerDropTargets([]client.Destination{
		fakeDestination{target: "https://agent-http-intake.logs.datadoghq.com/api/v2/logs"},
		fakeDestination{target: "https://http-intake.logs.datadoghq.eu:443/api/v2/logs"},
		fakeDestination{target: "https://mrf.logs.datadoghq.com/api/v2/logs", mrf: true},
	}, true)
	require.Len(t, reliable, 3)
	assert.Equal(t, metrics.DropTarget{Host: "agent-http-intake.logs.datadoghq.com", Kind: metrics.DropTargetPrimary}, reliable[0])
	assert.Equal(t, metrics.DropTarget{Host: "http-intake.logs.datadoghq.eu", Kind: metrics.DropTargetAdditional}, reliable[1])
	assert.Equal(t, metrics.DropTargetMRF, reliable[2].Kind)

	unreliable := workerDropTargets([]client.Destination{fakeDestination{target: "intake.example.com:10516"}}, false)
	assert.Equal(t, metrics.DropTarget{Host: "intake.example.com", Kind: metrics.DropTargetBestEffort}, unreliable[0])
}

func TestBatchRecordsMessageTooLarge(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	b := createTestBatch()
	output := make(chan *message.Payload, 10)
	b.processMessage(logMessage("this message is way too large for the content size limit"), output)

	got := metrics.DroppedLogsSnapshot()
	require.Len(t, got, 1)
	assert.Equal(t, metrics.DropReasonMessageTooLarge, got[0].Reason)
	assert.Equal(t, metrics.DropTargetAll, got[0].Kind)
	assert.Empty(t, got[0].Destination)
	assert.Equal(t, int64(1), got[0].Logs)
}

func TestBatchRecordsEncodeFailure(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	b := createTestBatch()
	b.serializer = &errorSerializer{}
	b.processMessage(logMessage("abc"), make(chan *message.Payload, 1))

	got := metrics.DroppedLogsSnapshot()
	require.Len(t, got, 1)
	assert.Equal(t, metrics.DropReasonEncodeFailed, got[0].Reason)
	assert.Equal(t, int64(1), got[0].Logs)
	assert.Equal(t, int64(3), got[0].Bytes)
}

// A message that fits is not a drop, whatever happens to the batch later.
func TestBatchDoesNotRecordDeliveredMessages(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	b := createTestBatch()
	output := make(chan *message.Payload, 10)
	b.processMessage(logMessage("ok"), output)
	b.flushBuffer(output, "flush")
	<-output

	assert.Empty(t, metrics.DroppedLogsSnapshot())
}

// stuckDestination accepts payloads into its buffer but never sends them until
// released, like an endpoint stuck retrying.
type stuckDestination struct {
	target  string
	started chan chan bool
	release chan struct{}
}

func (d *stuckDestination) IsMRF() bool    { return false }
func (d *stuckDestination) Target() string { return d.target }
func (d *stuckDestination) Metadata() *client.DestinationMetadata {
	return client.NewNoopDestinationMetadata()
}
func (d *stuckDestination) Start(input chan *message.Payload, _ chan *message.Payload, isRetrying chan bool) <-chan struct{} {
	stop := make(chan struct{})
	d.started <- isRetrying
	go func() {
		<-d.release
		for range input {
		}
		close(stop)
	}()
	return stop
}

// drainDestination delivers every payload at once.
type drainDestination struct{ target string }

func (d *drainDestination) IsMRF() bool    { return false }
func (d *drainDestination) Target() string { return d.target }
func (d *drainDestination) Metadata() *client.DestinationMetadata {
	return client.NewNoopDestinationMetadata()
}
func (d *drainDestination) Start(input chan *message.Payload, output chan *message.Payload, _ chan bool) <-chan struct{} {
	stop := make(chan struct{})
	go func() {
		for p := range input {
			output <- p
		}
		close(stop)
	}()
	return stop
}

// The main endpoint is stuck while a reliable additional endpoint takes the logs:
// once the main endpoint's buffer is full, it loses them, and that is recorded
// against the main endpoint, not the endpoint that delivered.
func TestWorkerRecordsBufferFullAgainstTheLaggingEndpoint(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	primary := &stuckDestination{target: "https://primary.example.com/api/v2/logs", started: make(chan chan bool, 1), release: make(chan struct{})}
	additional := &drainDestination{target: "https://copy.example.com/api/v2/logs"}
	factory := func(_ string) *client.Destinations {
		return client.NewDestinations([]client.Destination{primary, additional}, nil)
	}
	input := make(chan *message.Payload)
	auditor := &testAuditor{output: make(chan *message.Payload, 10)}
	w := newWorker(configmock.New(t), input, auditor, factory, 1, NewMockServerlessMeta(false), metrics.NewNoopPipelineMonitor(""), "test")
	w.start()
	retrying := <-primary.started

	payload := func() *message.Payload {
		return &message.Payload{MessageMetas: []*message.MessageMetadata{&logMessage("line").MessageMetadata}, Encoded: []byte("x")}
	}
	// Fills the main endpoint's one-slot buffer; both endpoints take it.
	input <- payload()
	<-auditor.output
	// The main endpoint starts retrying, so the next payload only reaches the
	// additional endpoint and finds the main endpoint's buffer full.
	retrying <- true
	input <- payload()
	<-auditor.output

	require.Eventually(t, func() bool { return len(metrics.DroppedLogsSnapshot()) == 1 }, 5*time.Second, 10*time.Millisecond)
	got := metrics.DroppedLogsSnapshot()[0]
	assert.Equal(t, metrics.DropReasonBufferFull, got.Reason)
	assert.Equal(t, metrics.DropTargetPrimary, got.Kind)
	assert.Equal(t, "primary.example.com", got.Destination)
	assert.Equal(t, int64(1), got.Logs)

	close(primary.release)
	w.stop()
}
