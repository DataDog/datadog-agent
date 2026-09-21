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

// FakeCore is an in-process Core used by driver tests. It seals one payload
// per accepted record, fans that payload out to every sender, and resolves
// durability when every reliable sender has acknowledged.
type FakeCore struct {
	classes         []SenderClass
	maxInflight     int
	maxPayloadBytes int

	ingestHeld atomic.Bool
	notifyHeld atomic.Bool
	senderHeld []atomic.Bool

	mu sync.Mutex

	started      bool
	shuttingDown bool
	abandoned    bool
	closed       bool
	inflight     int
	nextStream   uint64
	nextBatch    []uint32

	streams   []StreamID
	streamGen []uint64 // last opened generation, 0 if never opened

	queued   [][]queuedSend
	effects  [][]Effect
	released atomic.Int64

	pending       map[uint64]*fakePayload // keyed by the single metadata id
	order         []uint64
	notifications []Notification

	clockNanos uint64
}

type queuedSend struct {
	id      uint64
	payload []byte
}

type fakePayload struct {
	id           uint64
	body         []byte
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

// Start requests one stream per sender.
func (c *FakeCore) Start() Progress {
	c.enterIngest()
	defer c.leaveIngest()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = true
	wake := make([]SenderID, 0, len(c.classes))
	for i := range c.classes {
		c.nextStream++
		sid := StreamID(c.nextStream)
		c.effects[i] = append(c.effects[i], Effect{
			Kind:   OpenStream,
			Sender: SenderID(i),
			Stream: sid,
		})
		wake = append(wake, SenderID(i))
	}
	return c.progressLocked(wake)
}

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

// PushLog offers one record.
func (c *FakeCore) PushLog(record Record, nowNanos uint64, metadataID uint64) (Admission, Progress) {
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
	c.inflight++
	body := append([]byte(nil), record.Body...)
	p := &fakePayload{
		id:      metadataID,
		body:    body,
		acked:   make([]bool, len(c.classes)),
		dropped: make([]bool, len(c.classes)),
		batchOn: make([]uint32, len(c.classes)),
	}
	for i, class := range c.classes {
		if class == Reliable {
			p.reliableLeft++
		}
		_ = i
	}
	c.pending[metadataID] = p
	c.order = append(c.order, metadataID)

	wake := make([]SenderID, 0, len(c.classes))
	for i := range c.classes {
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
			Kind:        PayloadDropped,
			Abandoned:   true,
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

// PollSender drains one sender's effects.
func (c *FakeCore) PollSender(sender SenderID) []Effect {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.effects[sender]
	c.effects[sender] = nil
	return out
}

// HandleStreamOpened reports that a sender's stream is up.
func (c *FakeCore) HandleStreamOpened(sender SenderID, stream StreamID) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
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
// acknowledging it. Unreliable drops never resolve the auditor.
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
		p.dropped[sender] = true
		c.notifications = append(c.notifications, Notification{
			Kind:        PayloadDropped,
			Sender:      sender,
			HasSender:   true,
			Abandoned:   abandoned,
			MetadataIDs: []uint64{p.id},
		})
		if abandoned && c.classes[sender] == Reliable {
			c.resolveLocked(p, Notification{
				Kind:        PayloadDropped,
				Sender:      sender,
				HasSender:   true,
				Abandoned:   true,
				MetadataIDs: []uint64{p.id},
			})
		}
	}
	return c.progressLocked(nil)
}

// HandleStreamError reports that a sender's stream died.
func (c *FakeCore) HandleStreamError(sender SenderID, stream StreamID, message string) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streams[sender] != stream {
		return c.progressLocked(nil)
	}
	c.streamGen[sender] = 0
	c.streams[sender] = 0
	c.nextStream++
	sid := StreamID(c.nextStream)
	c.effects[sender] = append(c.effects[sender], Effect{
		Kind:   ReportError,
		Sender: sender,
		Stream: stream,
		Err:    &CoreError{Kind: StreamFailed, Message: message},
	}, Effect{
		Kind:   OpenStream,
		Sender: sender,
		Stream: sid,
	})
	return c.progressLocked([]SenderID{sender})
}

// HandleTimer feeds back a timer the library asked for.
func (c *FakeCore) HandleTimer(sender SenderID, stream StreamID, _ TimerKind) Progress {
	c.enterSender(sender)
	defer c.leaveSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.streams[sender] != stream {
		return c.progressLocked(nil)
	}
	return c.progressLocked(nil)
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
