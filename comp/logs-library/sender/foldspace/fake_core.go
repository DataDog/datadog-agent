// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// fakeRedialBackoff is how long FakeCore asks a sender to wait before
// redialing after a failure. It is non-zero because a driver waits on stop
// only while an OpenStream's After runs, so a dial failing at once on every
// attempt would otherwise never yield to Stop.
const fakeRedialBackoff = 10 * time.Millisecond

// FakeCore is an in-process Core used by driver tests. It seals one payload
// per accepted record, fans that payload out to every sender, and resolves
// durability when every reliable sender has acknowledged.
//
// A sender dials from the PollSender that finds it holding a queued payload,
// and a stream that ends with nothing outstanding and nothing queued closes
// without a redial, as the library's do.
type FakeCore struct {
	classes         []SenderClass
	maxInflight     int
	maxPayloadBytes int

	ingestHeld atomic.Bool
	notifyHeld atomic.Bool
	senderHeld []atomic.Bool

	mu sync.Mutex

	shuttingDown bool
	abandoned    bool
	closed       bool
	inflight     int
	nextStream   uint64
	nextBatch    []uint32

	streams   []StreamID
	streamGen []uint64   // last opened generation, 0 if never opened
	dialing   []StreamID // the stream an OpenStream asked for and no report has settled, 0 if none

	queued   [][]queuedSend
	effects  [][]Effect
	released atomic.Int64

	pending       map[uint64]*fakePayload // keyed by the single metadata id
	order         []uint64
	notifications []Notification
	timers        []TimerCall

	clockNanos uint64
}

type queuedSend struct {
	id      uint64
	payload []byte
}

type fakePayload struct {
	id           uint64
	body         []byte
	route        Route
	acked        []bool
	dropped      []bool
	batchOn      []uint32
	resolved     bool
	reliableLeft int
}

// FakeCoreConfig tunes FakeCore construction.
type FakeCoreConfig struct {
	Classes         []SenderClass
	MaxInflight     int
	MaxPayloadBytes int
}

// NewFakeCore returns a FakeCore. At least one sender must be reliable.
func NewFakeCore(cfg FakeCoreConfig) *FakeCore {
	if cfg.MaxInflight <= 0 {
		cfg.MaxInflight = 16
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = 256 * 1024
	}
	if len(cfg.Classes) == 0 {
		cfg.Classes = []SenderClass{Reliable}
	}
	n := len(cfg.Classes)
	c := &FakeCore{
		classes:         cfg.Classes,
		maxInflight:     cfg.MaxInflight,
		maxPayloadBytes: cfg.MaxPayloadBytes,
		senderHeld:      make([]atomic.Bool, n),
		nextBatch:       make([]uint32, n),
		streams:         make([]StreamID, n),
		streamGen:       make([]uint64, n),
		dialing:         make([]StreamID, n),
		queued:          make([][]queuedSend, n),
		effects:         make([][]Effect, n),
		pending:         make(map[uint64]*fakePayload),
	}
	return c
}

func (c *FakeCore) enterIngest() {
	if !c.ingestHeld.CompareAndSwap(false, true) {
		panic("foldspace: two callers in the ingest region at once")
	}
}

func (c *FakeCore) leaveIngest() { c.ingestHeld.Store(false) }

func (c *FakeCore) enterNotify() {
	if !c.notifyHeld.CompareAndSwap(false, true) {
		panic("foldspace: two callers in the notification region at once")
	}
}

func (c *FakeCore) leaveNotify() { c.notifyHeld.Store(false) }

func (c *FakeCore) enterSender(sender SenderID) {
	c.checkSender(sender)
	if !c.senderHeld[sender].CompareAndSwap(false, true) {
		panic(fmt.Sprintf("foldspace: two callers in the sender %d region at once", sender))
	}
}

func (c *FakeCore) leaveSender(sender SenderID) { c.senderHeld[sender].Store(false) }

func (c *FakeCore) checkSender(sender SenderID) {
	if int(sender) >= len(c.classes) {
		panic(fmt.Sprintf("foldspace: sender %d, but the client has %d", sender, len(c.classes)))
	}
}

// SenderCount is the number of senders.
func (c *FakeCore) SenderCount() int { return len(c.classes) }

// SenderClass returns the class of sender.
func (c *FakeCore) SenderClass(sender SenderID) SenderClass {
	c.checkSender(sender)
	return c.classes[sender]
}

// ReleasedLeases is how many leases have been released.
func (c *FakeCore) ReleasedLeases() int64 { return c.released.Load() }

// HasCapacity is whether a record can be offered.
func (c *FakeCore) HasCapacity() bool {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasCapacityLocked()
}

func (c *FakeCore) hasCapacityLocked() bool {
	return !c.shuttingDown && !c.abandoned && c.inflight < c.maxInflight
}

// PushLog offers one record. Only senders named in route receive it; a
// reliable sender excluded from route contributes nothing to reliableLeft, or
// the payload would wait forever on an ack from a sender it was never sent to.
func (c *FakeCore) PushLog(record Record, nowNanos uint64, metadataID uint64, route Route) (Admission, Progress) {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clockNanos = nowNanos
	if c.shuttingDown || c.abandoned {
		return ShuttingDown, c.progressLocked(nil)
	}
	if len(record.Body) > c.maxPayloadBytes {
		return TooLarge, c.progressLocked(nil)
	}
	if !c.hasCapacityLocked() {
		return Refused, c.progressLocked(nil)
	}
	if route&^AllSenders(len(c.classes)) != 0 {
		return Unroutable, c.progressLocked(nil)
	}
	if route == 0 {
		return Unrouted, c.progressLocked(nil)
	}
	if len(record.Body) == 0 {
		return Empty, c.progressLocked(nil)
	}
	c.inflight++
	body := append([]byte(nil), record.Body...)
	p := &fakePayload{
		id:      metadataID,
		body:    body,
		route:   route,
		acked:   make([]bool, len(c.classes)),
		dropped: make([]bool, len(c.classes)),
		batchOn: make([]uint32, len(c.classes)),
	}
	for i, class := range c.classes {
		if !route.Contains(SenderID(i)) {
			continue
		}
		if class == Reliable {
			p.reliableLeft++
		}
	}
	c.pending[metadataID] = p
	c.order = append(c.order, metadataID)

	wake := make([]SenderID, 0, len(c.classes))
	for i := range c.classes {
		if !route.Contains(SenderID(i)) {
			continue
		}
		c.enqueueSendLocked(SenderID(i), metadataID, body)
		wake = append(wake, SenderID(i))
	}
	return Accepted, c.progressLocked(wake)
}

func (c *FakeCore) enqueueSendLocked(sender SenderID, id uint64, body []byte) {
	if c.streamGen[sender] == 0 {
		c.queued[sender] = append(c.queued[sender], queuedSend{id: id, payload: body})
		return
	}
	c.queueSendLocked(sender, id, body)
}

func (c *FakeCore) queueSendLocked(sender SenderID, id uint64, body []byte) {
	c.nextBatch[sender]++
	batchID := c.nextBatch[sender]
	if p := c.pending[id]; p != nil {
		p.batchOn[sender] = batchID
	}
	lease := NewLease(body, func() { c.released.Add(1) })
	c.effects[sender] = append(c.effects[sender], Effect{
		Kind:    SendBatch,
		Sender:  sender,
		Stream:  c.streams[sender],
		BatchID: batchID,
		Batch:   lease,
	})
}

// Flush is a no-op besides returning capacity: FakeCore seals on each PushLog.
func (c *FakeCore) Flush() (Admission, Progress) {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shuttingDown || c.abandoned {
		return ShuttingDown, c.progressLocked(nil)
	}
	return Accepted, c.progressLocked(nil)
}

// BeginShutdown closes admission.
func (c *FakeCore) BeginShutdown() (Admission, Progress) {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shuttingDown = true
	return ShuttingDown, c.progressLocked(nil)
}

// Abandon resolves every seated record as an abandoning drop.
func (c *FakeCore) Abandon() Progress {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.abandoned = true
	c.shuttingDown = true
	ids := make([]uint64, 0, len(c.pending))
	for _, id := range c.order {
		p := c.pending[id]
		if p == nil || p.resolved {
			continue
		}
		p.resolved = true
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		c.notifications = append(c.notifications, Notification{
			Kind:        PayloadAbandoned,
			MetadataIDs: ids,
		})
		c.inflight = 0
	}
	return c.progressLocked(nil)
}

// IsDrained is whether everything admitted has been resolved.
func (c *FakeCore) IsDrained() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight == 0 && len(c.notifications) == 0
}

// PollSender drains one sender's effects, and dials for a sender holding no
// stream once it has a queued payload it still owes.
func (c *FakeCore) PollSender(sender SenderID) []Effect {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.effects[sender]
	c.effects[sender] = nil
	if c.streams[sender] == 0 && c.dialing[sender] == 0 && c.owesQueuedLocked(sender) {
		out = append(out, c.dialLocked(sender, 0))
	}
	return out
}

// dialLocked asks for a new stream for sender, after waiting after.
func (c *FakeCore) dialLocked(sender SenderID, after time.Duration) Effect {
	c.nextStream++
	c.dialing[sender] = StreamID(c.nextStream)
	return Effect{Kind: OpenStream, Sender: sender, Stream: c.dialing[sender], After: after}
}

// owesQueuedLocked is whether sender has a queued payload it still owes,
// dropping the queued ids it no longer does.
func (c *FakeCore) owesQueuedLocked(sender SenderID) bool {
	kept := c.queued[sender][:0]
	for _, q := range c.queued[sender] {
		if p := c.pending[q.id]; p != nil && !p.resolved && !p.acked[sender] && !p.dropped[sender] {
			kept = append(kept, q)
		}
	}
	c.queued[sender] = kept
	return len(kept) > 0
}

// busyLocked is whether sender has work its stream ending would lose: a sent
// payload not yet acknowledged, a send not yet polled, or a queued payload.
func (c *FakeCore) busyLocked(sender SenderID) bool {
	for _, effect := range c.effects[sender] {
		if effect.Kind == SendBatch {
			return true
		}
	}
	for _, id := range c.order {
		p := c.pending[id]
		if p != nil && !p.resolved && p.batchOn[sender] != 0 && !p.acked[sender] && !p.dropped[sender] {
			return true
		}
	}
	return c.owesQueuedLocked(sender)
}

// HandleStreamOpened reports that a sender's stream is up. A stream other than
// the one the sender is dialing is discarded.
func (c *FakeCore) HandleStreamOpened(sender SenderID, stream StreamID) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dialing[sender] != stream {
		return c.progressLocked(nil)
	}
	c.dialing[sender] = 0
	c.streams[sender] = stream
	c.streamGen[sender] = uint64(stream)
	queued := c.queued[sender]
	c.queued[sender] = nil
	for _, q := range queued {
		c.queueSendLocked(sender, q.id, q.payload)
	}
	wake := []SenderID{sender}
	if len(queued) == 0 {
		wake = nil
	}
	return c.progressLocked(wake)
}

// HandleAck reports one batch's outcome. A stale StreamID is discarded.
func (c *FakeCore) HandleAck(sender SenderID, stream StreamID, batchID uint32, status int32) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streams[sender] != stream {
		return c.progressLocked(nil)
	}
	if status != AckOK {
		c.effects[sender] = append(c.effects[sender], Effect{
			Kind:   ReportError,
			Sender: sender,
			Stream: stream,
			Err:    &CoreError{Kind: BatchRejected, BatchID: batchID, BatchStatus: status},
		})
		return c.progressLocked([]SenderID{sender})
	}
	for _, id := range c.order {
		p := c.pending[id]
		if p == nil || p.resolved || p.batchOn[sender] != batchID {
			continue
		}
		c.ackLocked(sender, p)
		break
	}
	return c.progressLocked(nil)
}

func (c *FakeCore) ackLocked(sender SenderID, p *fakePayload) {
	if p.acked[sender] || p.dropped[sender] {
		return
	}
	p.acked[sender] = true
	if c.classes[sender] == Reliable {
		p.reliableLeft--
		if p.reliableLeft == 0 {
			c.resolveLocked(p, Notification{
				Kind:        PayloadDurable,
				MetadataIDs: []uint64{p.id},
			})
		}
	}
}

func (c *FakeCore) resolveLocked(p *fakePayload, n Notification) {
	if p.resolved {
		return
	}
	p.resolved = true
	c.inflight--
	c.notifications = append(c.notifications, n)
}

// Drop marks a sender as having given up on an in-flight payload without
// acknowledging it. A non-abandoning drop is telemetry only: it names no
// record and never resolves the auditor. An abandoning drop from a reliable
// sender is a resolution: it names the record and ends its life.
func (c *FakeCore) Drop(sender SenderID, stream StreamID, abandoned bool) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streams[sender] != stream {
		return c.progressLocked(nil)
	}
	for _, id := range c.order {
		p := c.pending[id]
		if p == nil || p.resolved || p.acked[sender] || p.dropped[sender] {
			continue
		}
		if !p.route.Contains(sender) {
			continue
		}
		p.dropped[sender] = true
		if !abandoned {
			c.notifications = append(c.notifications, Notification{
				Kind:      DroppedStats,
				Sender:    sender,
				HasSender: true,
				Records:   1,
				Bytes:     uint64(len(p.body)),
			})
			continue
		}
		c.notifications = append(c.notifications, Notification{
			Kind:        PayloadAbandoned,
			Sender:      sender,
			HasSender:   true,
			MetadataIDs: []uint64{p.id},
		})
		if c.classes[sender] == Reliable {
			c.resolveLocked(p, Notification{
				Kind:        PayloadAbandoned,
				Sender:      sender,
				HasSender:   true,
				MetadataIDs: []uint64{p.id},
			})
		}
	}
	return c.progressLocked(nil)
}

// HandleStreamError reports that a sender's dial failed or its stream ended.
// A failed dial, or a stream that ends holding work, is reported as a
// StreamFailed and redialed after fakeRedialBackoff. A stream that ends with
// nothing to carry closes idle, and the sender dials again only once it has
// work.
func (c *FakeCore) HandleStreamError(sender SenderID, stream StreamID, message string) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case stream != 0 && stream == c.dialing[sender]:
		c.dialing[sender] = 0
	case stream != 0 && stream == c.streams[sender]:
		c.streamGen[sender] = 0
		c.streams[sender] = 0
		if !c.busyLocked(sender) {
			return c.progressLocked(nil)
		}
	default:
		return c.progressLocked(nil)
	}
	c.effects[sender] = append(c.effects[sender], Effect{
		Kind:   ReportError,
		Sender: sender,
		Stream: stream,
		Err:    &CoreError{Kind: StreamFailed, Message: message},
	}, c.dialLocked(sender, fakeRedialBackoff))
	return c.progressLocked([]SenderID{sender})
}

// HandleTimer feeds back a timer. RotateStream on the current stream closes it
// and queues the sends not yet polled, so the next poll dials a new stream for
// them, or none if nothing was waiting, as the library does once a rotating
// stream has drained. It does not drain: a batch still unacked on the rotated
// stream is never resolved.
func (c *FakeCore) HandleTimer(sender SenderID, stream StreamID, kind TimerKind) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timers = append(c.timers, TimerCall{Sender: sender, Stream: stream, Kind: kind})
	if c.streams[sender] != stream || kind != RotateStream {
		return c.progressLocked(nil)
	}
	kept := make([]Effect, 0, len(c.effects[sender]))
	for _, effect := range c.effects[sender] {
		if effect.Kind != SendBatch || effect.Stream != stream {
			kept = append(kept, effect)
			continue
		}
		for _, id := range c.order {
			if p := c.pending[id]; p != nil && !p.resolved && p.batchOn[sender] == effect.BatchID {
				c.queued[sender] = append(c.queued[sender], queuedSend{id: id, payload: p.body})
				break
			}
		}
		effect.Batch.Release()
	}
	c.streamGen[sender] = 0
	c.streams[sender] = 0
	c.effects[sender] = append(kept, Effect{
		Kind:   CloseStream,
		Sender: sender,
		Stream: stream,
	})
	return c.progressLocked([]SenderID{sender})
}

// TimerCall is one HandleTimer call.
type TimerCall struct {
	Sender SenderID
	Stream StreamID
	Kind   TimerKind
}

// Timers returns every HandleTimer call, in call order.
func (c *FakeCore) Timers() []TimerCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TimerCall(nil), c.timers...)
}

// PollNotifications drains the outcomes waiting for the consumer.
func (c *FakeCore) PollNotifications() []Notification {
	c.enterNotify()
	defer c.leaveNotify()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.notifications
	c.notifications = nil
	return out
}

// TakeClockAdvance reports how far PushLog timestamps have moved.
func (c *FakeCore) TakeClockAdvance() time.Duration {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	return 0
}

// Close destroys the client.
func (c *FakeCore) Close() {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// WithIngestHeld runs fn while the ingest region is held. fn panics if it
// enters ingest.
func (c *FakeCore) WithIngestHeld(fn func()) {
	c.enterIngest()
	defer c.leaveIngest()
	fn()
}

func (c *FakeCore) progressLocked(wake []SenderID) Progress {
	return Progress{
		Wake:               append([]SenderID(nil), wake...),
		HasCapacity:        c.hasCapacityLocked(),
		NotificationsReady: len(c.notifications) > 0,
	}
}
