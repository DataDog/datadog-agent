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
// per accepted record and fans that payload out to every sender on its route.
//
// The reliable senders on a route form one reliable set, as the library's
// FOLDSPACE_SENDER_RELIABLE_SET members do: the first one's acknowledgement
// makes the payload durable, and the payload is abandoned only once every one
// of them has given it up. A route with no reliable sender carries no promise,
// so its payload is durable at seal. A durable payload is still sent by every
// sender that owes it, and their acknowledgements are counted.
//
// A payload frees its capacity when it resolves, so IsDrained can report true
// while a sender still owes a durable payload. The library holds it until
// every sender that owes it has settled.
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
	// givingUp is set by HandleStreamAbandoned and cleared when a stream opens.
	givingUp []bool

	queued   [][]queuedSend
	effects  [][]Effect
	released atomic.Int64

	pending       map[uint64]*fakePayload // keyed by the single metadata id
	order         []uint64
	notifications []Notification
	counts        []SenderCounts

	clockNanos uint64
}

type queuedSend struct {
	id      uint64
	payload []byte
}

type fakePayload struct {
	id      uint64
	body    []byte
	route   Route
	acked   []bool
	dropped []bool
	givenUp []bool // dropped by an abandoning give-up
	batchOn []uint32
	// resolved is whether the payload's notification has been emitted.
	resolved bool
	// abandoned is whether it resolved as abandoned, after which no sender
	// owes it.
	abandoned bool
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
		givingUp:        make([]bool, n),
		counts:          make([]SenderCounts, n),
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

// PushLog offers one record. Only senders named in route receive it, and only
// the reliable senders in route form the set whose acknowledgement makes it
// durable: one excluded from route is never sent it and cannot settle it.
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
		givenUp: make([]bool, len(c.classes)),
		batchOn: make([]uint32, len(c.classes)),
	}
	c.pending[metadataID] = p
	c.order = append(c.order, metadataID)

	wake := make([]SenderID, 0, len(c.classes))
	for i := range c.classes {
		if !route.Contains(SenderID(i)) {
			continue
		}
		if c.givingUp[i] {
			c.giveUpLocked(SenderID(i), p)
			continue
		}
		c.enqueueSendLocked(SenderID(i), metadataID, body)
		wake = append(wake, SenderID(i))
	}
	if !c.routesReliableLocked(route) {
		c.resolveLocked(p, Notification{Kind: PayloadDurable, MetadataIDs: []uint64{p.id}})
	}
	return Accepted, c.progressLocked(wake)
}

// routesReliableLocked is whether route names a reliable sender.
func (c *FakeCore) routesReliableLocked(route Route) bool {
	for i, class := range c.classes {
		if class == Reliable && route.Contains(SenderID(i)) {
			return true
		}
	}
	return false
}

// owedLocked is whether sender still has to carry p: it is on p's route, has
// neither acknowledged nor dropped it, and p was not abandoned. A durable
// payload stays owed to the senders that have not settled it.
func (c *FakeCore) owedLocked(sender SenderID, p *fakePayload) bool {
	return !p.abandoned && p.route.Contains(sender) && !p.acked[sender] && !p.dropped[sender]
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
		if p == nil {
			continue
		}
		p.abandoned = true
		if p.resolved {
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
		if p := c.pending[q.id]; p != nil && c.owedLocked(sender, p) {
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
		if p != nil && p.batchOn[sender] != 0 && c.owedLocked(sender, p) {
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
	c.givingUp[sender] = false
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
		if p == nil || p.batchOn[sender] != batchID {
			continue
		}
		c.ackLocked(sender, p)
		break
	}
	return c.progressLocked(nil)
}

// ackLocked counts sender's acknowledgement of p. The first reliable one makes
// p durable; any later one only counts.
func (c *FakeCore) ackLocked(sender SenderID, p *fakePayload) {
	if !c.owedLocked(sender, p) {
		return
	}
	p.acked[sender] = true
	c.counts[sender].SentRecords++
	c.counts[sender].SentBytes += uint64(len(p.body))
	if c.classes[sender] == Reliable {
		c.resolveLocked(p, Notification{
			Kind:        PayloadDurable,
			MetadataIDs: []uint64{p.id},
		})
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

// Drop marks a sender as having dropped every payload it owes without
// acknowledging it, and counts each in the sender's stats. A non-abandoning
// drop resolves nothing. An abandoning drop gives each payload up, which
// abandons one only once every reliable sender on its route has given it up.
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
		if p == nil || !c.owedLocked(sender, p) {
			continue
		}
		if abandoned {
			c.giveUpLocked(sender, p)
			continue
		}
		c.dropLocked(sender, p)
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

// HandleStreamAbandoned reports that a sender's stream cannot be opened and is
// not expected to become openable. It fails the stream like HandleStreamError,
// and the sender gives up every payload it owes, as it does what seals for it
// until a stream opens. Each is counted dropped. A payload is abandoned once
// every reliable sender on its route has given it up; while another reliable
// sender still owes it, that sender's acknowledgement can still make it
// durable.
func (c *FakeCore) HandleStreamAbandoned(sender SenderID, stream StreamID, message string) Progress {
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
	default:
		return c.progressLocked(nil)
	}
	c.givingUp[sender] = true
	c.queued[sender] = nil
	kept := c.effects[sender][:0]
	for _, effect := range c.effects[sender] {
		if effect.Kind == SendBatch {
			effect.Batch.Release()
			continue
		}
		kept = append(kept, effect)
	}
	c.effects[sender] = kept
	for _, id := range c.order {
		if p := c.pending[id]; p != nil && p.route.Contains(sender) {
			c.giveUpLocked(sender, p)
		}
	}
	c.effects[sender] = append(c.effects[sender], Effect{
		Kind:   ReportError,
		Sender: sender,
		Stream: stream,
		Err:    &CoreError{Kind: StreamFailed, Message: message},
	}, c.dialLocked(sender, fakeRedialBackoff))
	return c.progressLocked([]SenderID{sender})
}

// giveUpLocked drops p for sender as an abandoning give-up, and abandons p if
// that leaves no reliable sender on its route that has not given it up.
func (c *FakeCore) giveUpLocked(sender SenderID, p *fakePayload) {
	if !c.owedLocked(sender, p) {
		return
	}
	c.dropLocked(sender, p)
	p.givenUp[sender] = true
	if p.resolved || c.classes[sender] != Reliable {
		return
	}
	for i, class := range c.classes {
		if class == Reliable && p.route.Contains(SenderID(i)) && !p.givenUp[i] {
			return
		}
	}
	p.abandoned = true
	c.resolveLocked(p, Notification{
		Kind:        PayloadAbandoned,
		MetadataIDs: []uint64{p.id},
	})
}

func (c *FakeCore) dropLocked(sender SenderID, p *fakePayload) {
	p.dropped[sender] = true
	c.counts[sender].DroppedRecords++
	c.counts[sender].DroppedBytes += uint64(len(p.body))
}

// SenderStats reports what sender has had acknowledged and given up on. Like
// the library's, it enters no sender's region.
func (c *FakeCore) SenderStats(sender SenderID) SenderCounts {
	c.checkSender(sender)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[sender]
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
