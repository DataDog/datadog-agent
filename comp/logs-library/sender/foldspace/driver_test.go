// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type channelSink struct {
	ch chan *message.Payload
}

func newChannelSink() *channelSink {
	return &channelSink{ch: make(chan *message.Payload, 64)}
}

func (s *channelSink) Channel() chan *message.Payload { return s.ch }

func testMessage(body string) *message.Message {
	src := sources.NewLogSource("test", &config.LogsConfig{Service: "svc", Source: "src"})
	origin := message.NewOrigin(src)
	msg := message.NewMessage([]byte(body), origin, "info", time.Now().UnixNano())
	msg.Hostname = "host"
	msg.SetRendered([]byte(body))
	return msg
}

func startDriver(t *testing.T, core Core, transport Transport, sink *channelSink, dualShip bool) *Driver {
	t.Helper()
	d := NewDriver(DriverOptions{
		Core:            core,
		Transport:       transport,
		Sink:            sink,
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		PipelineDepth:   4,
		ShutdownTimeout: 2 * time.Second,
		DualShip:        dualShip,
	})
	d.Start()
	t.Cleanup(d.Stop)
	return d
}

func waitPayloads(t *testing.T, sink *channelSink, n int) []*message.Payload {
	t.Helper()
	var out []*message.Payload
	deadline := time.After(2 * time.Second)
	for len(out) < n {
		select {
		case p := <-sink.ch:
			out = append(out, p)
		case <-deadline:
			t.Fatalf("timed out waiting for %d payloads, got %d", n, len(out))
		}
	}
	return out
}

func TestOfferOrderDurable(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	transport := NewFakeTransport(1)
	sink := newChannelSink()
	d := startDriver(t, core, transport, sink, false)

	for _, body := range []string{"one", "two", "three"} {
		d.Offer(testMessage(body))
	}
	payloads := waitPayloads(t, sink, 3)
	got := make([]string, 0, 3)
	for i := range payloads {
		require.Len(t, payloads[i].MessageMetas, 1)
		got = append(got, string(transport.Sent(0)[i]))
	}
	assert.Equal(t, []string{"one", "two", "three"}, got)
}

func TestRefuseThenRetry(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 1})
	progress := core.Start()
	require.Len(t, progress.Wake, 1)
	effects := core.PollSender(0)
	require.Equal(t, OpenStream, effects[0].Kind)
	core.HandleStreamOpened(0, effects[0].Stream)

	admission, _ := core.PushLog(Record{Body: []byte("first")}, 1, 1)
	require.Equal(t, Accepted, admission)
	effects = core.PollSender(0)
	require.Equal(t, SendBatch, effects[0].Kind)
	batchID := effects[0].BatchID
	effects[0].Batch.Release()

	admission, _ = core.PushLog(Record{Body: []byte("second")}, 2, 2)
	require.Equal(t, Refused, admission)

	core.HandleAck(0, effects[0].Stream, batchID, AckOK)
	require.Equal(t, PayloadDurable, core.PollNotifications()[0].Kind)

	admission, progress = core.PushLog(Record{Body: []byte("second")}, 3, 2)
	require.Equal(t, Accepted, admission)
	require.Contains(t, progress.Wake, SenderID(0))
}

func TestTooLargeSkipsAuditor(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxPayloadBytes: 4})
	transport := NewFakeTransport(1)
	sink := newChannelSink()
	d := startDriver(t, core, transport, sink, false)

	d.Offer(testMessage("toolarge"))
	payloads := waitPayloads(t, sink, 1)
	assert.Empty(t, transport.Sent(0))
	require.Len(t, payloads[0].MessageMetas, 1)
}

func TestLeaseReleasedOnce(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	transport := NewFakeTransport(1)
	sink := newChannelSink()
	d := startDriver(t, core, transport, sink, false)

	d.Offer(testMessage("lease"))
	waitPayloads(t, sink, 1)
	require.Eventually(t, func() bool { return core.ReleasedLeases() == 1 }, time.Second, 10*time.Millisecond)
}

func TestStaleStreamIDDiscarded(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	progress := core.Start()
	require.Len(t, progress.Wake, 1)
	effects := core.PollSender(0)
	require.Equal(t, OpenStream, effects[0].Kind)
	live := effects[0].Stream
	core.HandleStreamOpened(0, live)

	_, progress = core.PushLog(Record{Body: []byte("x")}, 1, 7)
	require.Contains(t, progress.Wake, SenderID(0))
	effects = core.PollSender(0)
	require.Equal(t, SendBatch, effects[0].Kind)
	effects[0].Batch.Release()

	// Stale generation is ignored.
	core.HandleAck(0, live+99, effects[0].BatchID, AckOK)
	assert.Empty(t, core.PollNotifications())

	core.HandleAck(0, live, effects[0].BatchID, AckOK)
	notes := core.PollNotifications()
	require.Len(t, notes, 1)
	assert.Equal(t, PayloadDurable, notes[0].Kind)
	assert.Equal(t, []uint64{7}, notes[0].MetadataIDs)
}

func TestShutdownAbandonResolvesOnce(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 8})
	transport := NewFakeTransport(1)
	transport.blockRecv = true
	sink := newChannelSink()
	d := NewDriver(DriverOptions{
		Core:            core,
		Transport:       transport,
		Sink:            sink,
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		ShutdownTimeout: 50 * time.Millisecond,
	})
	d.Start()
	d.Offer(testMessage("held"))
	require.Eventually(t, func() bool { return len(transport.Sent(0)) == 1 }, time.Second, 10*time.Millisecond)
	d.Stop()

	select {
	case <-sink.ch:
		t.Fatal("abandoned records must not be acked to the auditor")
	default:
	}
	assert.True(t, core.IsDrained())
}

func TestConcurrentIngestPanics(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	var panicked atomicBool
	core.WithIngestHeld(func() {
		defer func() {
			if recover() != nil {
				panicked.set()
			}
		}()
		core.HasCapacity()
	})
	assert.True(t, panicked.get())
}

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) set() {
	b.mu.Lock()
	b.v = true
	b.mu.Unlock()
}
func (b *atomicBool) get() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.v
}

func TestOnePayloadWakesEverySender(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable, Unreliable}})
	core.Start()
	for i := 0; i < 2; i++ {
		effects := core.PollSender(SenderID(i))
		require.Equal(t, OpenStream, effects[0].Kind)
		core.HandleStreamOpened(SenderID(i), effects[0].Stream)
	}
	_, progress := core.PushLog(Record{Body: []byte("fan")}, 1, 1)
	assert.ElementsMatch(t, []SenderID{0, 1}, progress.Wake)
}

func TestReliableAckResolvesUnreliableDropDoesNot(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable, Unreliable}})
	core.Start()
	var streams [2]StreamID
	for i := 0; i < 2; i++ {
		effects := core.PollSender(SenderID(i))
		require.Equal(t, OpenStream, effects[0].Kind)
		streams[i] = effects[0].Stream
		core.HandleStreamOpened(SenderID(i), streams[i])
	}
	_, _ = core.PushLog(Record{Body: []byte("shared")}, 1, 9)
	var reliableBatch uint32
	for i := 0; i < 2; i++ {
		effects := core.PollSender(SenderID(i))
		require.Equal(t, SendBatch, effects[0].Kind)
		if i == 0 {
			reliableBatch = effects[0].BatchID
		}
		effects[0].Batch.Release()
	}

	core.Drop(1, streams[1], false)
	notes := core.PollNotifications()
	require.Len(t, notes, 1)
	assert.Equal(t, PayloadDropped, notes[0].Kind)
	assert.False(t, notes[0].Abandoned)
	assert.Empty(t, core.PollNotifications())

	core.HandleAck(0, streams[0], reliableBatch, AckOK)
	notes = core.PollNotifications()
	require.Len(t, notes, 1)
	assert.Equal(t, PayloadDurable, notes[0].Kind)
	assert.Equal(t, []uint64{9}, notes[0].MetadataIDs)
}

func TestDualShipRefuseDoesNotStall(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 1})
	transport := NewFakeTransport(1)
	transport.blockRecv = true
	sink := newChannelSink()
	d := NewDriver(DriverOptions{
		Core:            core,
		Transport:       transport,
		Sink:            sink,
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		InputSize:       1,
		PipelineDepth:   4,
		ShutdownTimeout: 2 * time.Second,
		DualShip:        true,
	})
	d.Start()
	t.Cleanup(d.Stop)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 32; i++ {
			d.Offer(testMessage("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dual-ship offer blocked on foldspace back-pressure")
	}
	select {
	case <-sink.ch:
		t.Fatal("dual-ship must not write the auditor")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCloneBeforeEncode(t *testing.T) {
	tap := make(chan *message.Message, 1)
	inner := recordingEncoder{}
	enc := NewTeeEncoder(&inner, tap)
	msg := testMessage("body")
	require.NoError(t, enc.Encode(msg, "host"))
	clone := <-tap
	assert.Equal(t, []byte("body"), clone.GetContent())
	assert.Equal(t, []byte("encoded"), msg.GetContent())
	assert.NotEqual(t, &msg.MessageContent, &clone.MessageContent)
}

type recordingEncoder struct{}

func (recordingEncoder) Encode(msg *message.Message, _ string) error {
	msg.SetEncoded([]byte("encoded"))
	return nil
}
