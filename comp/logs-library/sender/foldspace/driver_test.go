// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"fmt"
	"sync"
	"sync/atomic"
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

// countingFlushCore counts Flush calls so a test can distinguish a periodic
// seal from the single one Stop performs.
type countingFlushCore struct {
	Core
	flushes atomic.Int64
}

func (c *countingFlushCore) Flush() (Admission, Progress) {
	c.flushes.Add(1)
	return c.Core.Flush()
}

// TestIngestFlushesOnTimer asserts ingest seals whatever the core holds on a
// timer. The core seals on record count and content size only, so without this
// a partial batch left by a lull waits until shutdown.
func TestIngestFlushesOnTimer(t *testing.T) {
	core := &countingFlushCore{Core: NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})}
	transport := NewFakeTransport(1)
	sink := newChannelSink()

	d := NewDriver(DriverOptions{
		Core:            core,
		Transport:       transport,
		Sink:            sink,
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		ShutdownTimeout: 2 * time.Second,
		BatchWait:       20 * time.Millisecond,
	})
	d.Start()
	defer d.Stop()

	// One record, far short of any size threshold, then nothing further: only a
	// timer can move it.
	d.Offer(testMessage("partial"))

	require.Eventually(t, func() bool { return core.flushes.Load() >= 3 }, 2*time.Second, 10*time.Millisecond,
		"ingest must keep sealing on the timer while the driver runs")
}

// TestBatchWaitDefaults asserts the ticker is always armed, since NewTicker
// panics on a non-positive interval.
func TestBatchWaitDefaults(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	d := NewDriver(DriverOptions{Core: core, Transport: NewFakeTransport(1)})
	assert.Positive(t, d.batchWait)
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

// Dual-ship names sink ownership: the primary destination owns the auditor, so
// this driver must not write it even once its records are durable.
func TestDualShipDoesNotWriteTheSink(t *testing.T) {
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
	transport := NewFakeTransport(1)
	sink := newChannelSink()
	d := startDriver(t, core, transport, sink, true)

	for _, body := range []string{"one", "two", "three"} {
		d.Offer(testMessage(body))
	}

	// The records reaching the transport is what makes the silent sink meaningful:
	// nothing was written because ownership says so, not because nothing shipped.
	deadline := time.After(2 * time.Second)
	for len(transport.Sent(0)) < 3 {
		select {
		case p := <-sink.ch:
			t.Fatalf("dual-ship must not write the auditor, got %d metas", len(p.MessageMetas))
		case <-deadline:
			t.Fatalf("only %d of 3 records shipped", len(transport.Sent(0)))
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// A full ingest buffer back-pressures the caller. Shedding there would trade a
// delay for a lost log, so there is no configuration in which it is preferable.
func TestOfferWaitsForCapacity(t *testing.T) {
	d := NewDriver(DriverOptions{
		Core:            NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 1}),
		Transport:       NewFakeTransport(1),
		Sink:            newChannelSink(),
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		InputSize:       1,
		PipelineDepth:   4,
		ShutdownTimeout: 2 * time.Second,
		DualShip:        true,
	})

	// Ingest is not running, so the buffer is the whole capacity.
	d.Offer(testMessage("first"))

	offered := make(chan struct{})
	go func() {
		d.Offer(testMessage("second"))
		close(offered)
	}()

	select {
	case <-offered:
		t.Fatal("offer must wait for capacity, not discard the record")
	case <-time.After(50 * time.Millisecond):
	}

	<-d.input
	select {
	case <-offered:
	case <-time.After(time.Second):
		t.Fatal("blocked offer did not proceed after capacity freed")
	}
}

// The tap runs before the primary encoder rewrites the message in place, so the
// snapshot must keep the rendered body independently of the message.
func TestTapSnapshotsRenderedBody(t *testing.T) {
	input := make(chan ingestItem, 1)
	tap := &DriverTap{input: input}
	msg := testMessage("body")

	tap.Tap(msg)
	item := <-input

	inner := recordingEncoder{}
	require.NoError(t, inner.Encode(msg, "host"))

	assert.Equal(t, []byte("body"), item.record.Body, "the snapshot must survive the primary encoder")
	assert.Equal(t, []byte("encoded"), msg.GetContent())

	// Dual-ship leaves the metadata behind: the primary destination owns the
	// auditor sink, so there is nothing for this driver to release.
	assert.Nil(t, item.meta)
}

// A full tap buffer means the pipeline is outrunning foldspace. Waiting there
// back-pressures the processor; the alternative would be to lose the log.
func TestTapWaitsForCapacity(t *testing.T) {
	input := make(chan ingestItem, 1)
	tap := &DriverTap{input: input}

	tap.Tap(testMessage("first"))

	tapped := make(chan struct{})
	go func() {
		tap.Tap(testMessage("second"))
		close(tapped)
	}()

	select {
	case <-tapped:
		t.Fatal("the tap must wait for capacity rather than shed")
	case <-time.After(50 * time.Millisecond):
	}

	<-input // free a slot; the blocked Tap now completes
	select {
	case <-tapped:
	case <-time.After(time.Second):
		t.Fatal("blocked Tap did not proceed after capacity freed")
	}
}

type recordingEncoder struct{}

func (recordingEncoder) Encode(msg *message.Message, _ string) error {
	msg.SetEncoded([]byte("encoded"))
	return nil
}

// The sender goroutine performs the sends and is also the only consumer of acks,
// so it must never wait mid-send for an ack: that would be waiting on itself, and
// the records queued behind it would never ship. Nothing in the sender may bound
// outstanding sends below max_inflight_payloads, which is why this sweeps
// pipeline_depth across values both under and over the window rather than
// checking the one that happened to be shipped.
func TestSenderNeverWaitsOnAnAckItOwes(t *testing.T) {
	const inflight = 16
	for _, depth := range []int{1, 2, 4, 8, 16, 32} {
		t.Run(fmt.Sprintf("pipeline_depth=%d", depth), func(t *testing.T) {
			core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: inflight})
			transport := NewFakeTransport(1)
			transport.blockRecv = true // no ack ever arrives, so every send stays outstanding
			d := NewDriver(DriverOptions{
				Core:            core,
				Transport:       transport,
				Sink:            newChannelSink(),
				PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
				InputSize:       inflight * 2,
				PipelineDepth:   depth,
				ShutdownTimeout: 200 * time.Millisecond,
			})
			d.Start()
			t.Cleanup(d.Stop)

			// FakeCore seals one payload per record, so a full window is one offer each.
			for i := 0; i < inflight; i++ {
				d.Offer(testMessage(fmt.Sprintf("record-%d", i)))
			}

			deadline := time.After(3 * time.Second)
			for len(transport.Sent(0)) < inflight {
				select {
				case <-deadline:
					t.Fatalf("only %d of %d payloads reached the transport", len(transport.Sent(0)), inflight)
				case <-time.After(5 * time.Millisecond):
				}
			}
		})
	}
}

// Steady state exercises the interleaving the window test cannot: acks arriving
// while further batches seal. The sender must keep draining effects across many
// windows rather than wedging once the first one fills.
func TestSenderSustainsManyWindows(t *testing.T) {
	const payloads = 64 // four times the default window
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 16})
	transport := NewFakeTransport(1)
	d := startDriver(t, core, transport, newChannelSink(), false)

	for i := 0; i < payloads; i++ {
		d.Offer(testMessage(fmt.Sprintf("record-%d", i)))
	}

	deadline := time.After(5 * time.Second)
	for len(transport.Sent(0)) < payloads {
		select {
		case <-deadline:
			t.Fatalf("only %d of %d payloads reached the transport", len(transport.Sent(0)), payloads)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// A wedged sender loses its queued sends when Stop releases it, and that loss is
// silent: an abandoning drop increments no counter. Stop must therefore deliver
// everything the core accepted.
func TestStopDeliversEverythingAccepted(t *testing.T) {
	const payloads = 32
	core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}, MaxInflight: 16})
	transport := NewFakeTransport(1)
	d := NewDriver(DriverOptions{
		Core:            core,
		Transport:       transport,
		Sink:            newChannelSink(),
		PipelineMonitor: metrics.NewNoopPipelineMonitor("test"),
		InputSize:       payloads,
		PipelineDepth:   4, // below the window, where a self-imposed bound would bind
		ShutdownTimeout: 5 * time.Second,
	})
	d.Start()

	for i := 0; i < payloads; i++ {
		d.Offer(testMessage(fmt.Sprintf("record-%d", i)))
	}
	d.Stop()

	assert.Len(t, transport.Sent(0), payloads, "Stop must not discard accepted records")
}
