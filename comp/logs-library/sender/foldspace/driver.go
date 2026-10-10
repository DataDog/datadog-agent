// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DriverOptions configures one Driver.
type DriverOptions struct {
	Core            Core
	Transport       Transport
	Sink            sender.Sink
	PipelineMonitor metrics.PipelineMonitor
	InputSize       int
	PipelineDepth   int
	ConnectTimeout  time.Duration
	// SendTimeout bounds each Stream.Send. A send that exceeds it fails the
	// stream, so an intake that stops granting flow-control window cannot hold
	// the sender goroutine, and every ack it owes, indefinitely.
	SendTimeout time.Duration
	// AckTimeout is how long a stream may hold unacknowledged batches without
	// any ack arriving before it is failed. It catches an intake whose
	// transport stays healthy but which never acks: the window fills, nothing
	// more is sent, and neither the send deadline nor keepalive fires.
	AckTimeout        time.Duration
	ShutdownTimeout   time.Duration
	StateRequestBytes int
	// BatchWait is how often ingest seals whatever the core is holding, bounding
	// how long a partial batch waits when no further records arrive to fill it.
	BatchWait time.Duration
	// DualShip, when true, means the primary destination owns the auditor sink,
	// so this driver must not release metadata to it.
	DualShip bool
	// Config reads multi_region_failover.enabled/failover_logs, mirroring
	// destination_sender.go's canSend() gate for the primary HTTP path. Nil
	// disables MRF routing regardless of MRFRoute.
	Config pkgconfigmodel.Reader
	// MRFRoute names the senders built from MRF endpoints. Zero means none:
	// every sender is routed unconditionally and mrfEnabled is never consulted.
	MRFRoute Route
}

// Driver consumes processor output, drives a Core, and fans payloads out
// through Transport. It implements sender.PipelineComponent so the pipeline
// provider can start and stop it with the rest of the logs agent.
type Driver struct {
	core      Core
	transport Transport
	sink      sender.Sink
	monitor   metrics.PipelineMonitor

	input           chan ingestItem
	pipelineDepth   int
	connectTimeout  time.Duration
	sendTimeout     time.Duration
	ackTimeout      time.Duration
	shutdownTimeout time.Duration
	batchWait       time.Duration
	dualShip        bool
	cfg             pkgconfigmodel.Reader
	mrfRoute        Route
	nonMRFRoute     Route

	pending    *pendingTable
	nextID     atomic.Uint64
	startNanos int64

	wake       []chan struct{}
	notifyWake chan struct{}
	capacity   chan struct{}

	stopOnce       sync.Once
	stop           chan struct{}
	stopped        chan struct{}
	ingestDone     chan struct{}
	ingestCancel   chan struct{}
	ingestStopping chan struct{}

	wg sync.WaitGroup
}

// ingestItem is one log queued for the core.
//
// It carries a Record rather than a *message.Message because a Record is
// self-contained: recordFromMessage copies the body and tags out, so the caller
// may keep mutating the message afterwards. That is what lets the dual-ship tap
// hand off without cloning a whole message.
//
// meta is the metadata to release once the record is durable, and is set only
// when this driver owns the auditor sink. Dual-ship leaves it nil: the primary
// destination owns the real message's metadata, so there is nothing here to
// release and nothing to track.
type ingestItem struct {
	record Record
	meta   *message.MessageMetadata
}

type streamAck struct {
	stream StreamID
	id     uint32
	status int32
	err    error
}

type scheduledTimer struct {
	stream StreamID
	kind   TimerKind
}

// NewDriver constructs a Driver. The Core must already be configured with the
// same sender count the Transport serves.
func NewDriver(opts DriverOptions) *Driver {
	n := opts.Core.SenderCount()
	if opts.InputSize <= 0 {
		opts.InputSize = 100
	}
	if opts.PipelineDepth <= 0 {
		opts.PipelineDepth = 8
	}
	if opts.ConnectTimeout <= 0 {
		opts.ConnectTimeout = 10 * time.Second
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = 10 * time.Second
	}
	if opts.AckTimeout <= 0 {
		opts.AckTimeout = 30 * time.Second
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 15 * time.Second
	}
	if opts.BatchWait <= 0 {
		opts.BatchWait = 5 * time.Second
	}
	if opts.PipelineMonitor == nil {
		opts.PipelineMonitor = metrics.NewNoopPipelineMonitor("foldspace")
	}
	wake := make([]chan struct{}, n)
	for i := range wake {
		wake[i] = make(chan struct{}, 1)
	}
	return &Driver{
		core:            opts.Core,
		transport:       opts.Transport,
		sink:            opts.Sink,
		monitor:         opts.PipelineMonitor,
		input:           make(chan ingestItem, opts.InputSize),
		pipelineDepth:   opts.PipelineDepth,
		connectTimeout:  opts.ConnectTimeout,
		sendTimeout:     opts.SendTimeout,
		ackTimeout:      opts.AckTimeout,
		shutdownTimeout: opts.ShutdownTimeout,
		batchWait:       opts.BatchWait,
		dualShip:        opts.DualShip,
		cfg:             opts.Config,
		mrfRoute:        opts.MRFRoute,
		nonMRFRoute:     AllSenders(n) &^ opts.MRFRoute,
		pending:         newPendingTable(),
		startNanos:      time.Now().UnixNano(),
		wake:            wake,
		notifyWake:      make(chan struct{}, 1),
		capacity:        make(chan struct{}, 1),
		stop:            make(chan struct{}),
		stopped:         make(chan struct{}),
		ingestDone:      make(chan struct{}),
		ingestCancel:    make(chan struct{}),
		ingestStopping:  make(chan struct{}),
	}
}

// In is unused: foldspace consumes *message.Message, not *message.Payload.
func (d *Driver) In() chan *message.Payload { return nil }

// PipelineMonitor returns the monitor shared with the processor.
func (d *Driver) PipelineMonitor() metrics.PipelineMonitor { return d.monitor }

// Tap returns a tap feeding this driver's ingest, for use when the primary
// destination owns the auditor sink and foldspace observes the rendered message
// alongside it.
func (d *Driver) Tap() *DriverTap { return &DriverTap{input: d.input, stopping: d.ingestStopping} }

// Offer sends msg to ingest, blocking while the buffer is full so that
// back-pressure reaches the processor rather than costing a record. It gives
// up once Stop has started, rather than ever racing a close of d.input: a
// shutdown that starts while this call is blocked must be able to release it
// without the send panicking.
//
// This driver owns the auditor sink here, so the message's metadata rides along
// to be released once the record is durable.
func (d *Driver) Offer(msg *message.Message) {
	select {
	case d.input <- ingestItem{record: recordFromMessage(msg), meta: &msg.MessageMetadata}:
	case <-d.ingestStopping:
	}
}

// Start launches ingest, per-sender workers, and the notification drain.
func (d *Driver) Start() {
	d.monitor.Start()
	progress := d.core.Start()
	d.dispatch(progress)

	d.wg.Add(1)
	go d.ingestLoop()

	for i := 0; i < d.core.SenderCount(); i++ {
		d.wg.Add(1)
		go d.senderLoop(SenderID(i))
	}

	d.wg.Add(1)
	go d.notifyLoop()
}

// Stop flushes, drains until the deadline, then abandons anything remaining.
func (d *Driver) Stop() {
	d.stopOnce.Do(func() {
		// Closing d.ingestStopping, not d.input, is what lets a concurrently
		// blocked Offer/Tap release safely: see their doc comments.
		close(d.ingestStopping)
		select {
		case <-d.ingestDone:
		case <-time.After(d.shutdownTimeout):
			close(d.ingestCancel)
			<-d.ingestDone
		}

		deadline := time.After(d.shutdownTimeout)
		_, progress := d.core.Flush()
		d.dispatch(progress)
		_, progress = d.core.BeginShutdown()
		d.dispatch(progress)

		waitDrained := func(bound <-chan time.Time) bool {
			for {
				if d.core.IsDrained() {
					return true
				}
				select {
				case <-bound:
					return d.core.IsDrained()
				case <-time.After(10 * time.Millisecond):
				}
			}
		}

		if !waitDrained(deadline) {
			progress = d.core.Abandon()
			d.dispatch(progress)
			_ = waitDrained(time.After(time.Second))
		}
		close(d.stop)
		d.wg.Wait()
		if closer, ok := d.transport.(interface{ Close() }); ok {
			closer.Close()
		}
		d.core.Close()
		d.monitor.Stop()
		close(d.stopped)
	})
	<-d.stopped
}

// DriverGroup presents a set of Drivers as the single sender.PipelineComponent
// the pipeline provider starts and stops.
//
// Each pipeline owns one Driver, and each Driver one Core, because a Core admits
// a single ingest caller at a time: stateful encoding parallelizes only by
// running parallel cores. The drivers share a pipeline monitor so the provider
// still reports one set of component snapshots.
type DriverGroup struct {
	drivers []*Driver
	monitor metrics.PipelineMonitor
}

var _ sender.PipelineComponent = (*DriverGroup)(nil)

// NewDriverGroup returns a group over drivers, which must be non-empty and must
// share a pipeline monitor.
func NewDriverGroup(drivers []*Driver) *DriverGroup {
	return &DriverGroup{drivers: drivers, monitor: drivers[0].PipelineMonitor()}
}

// Drivers returns the drivers in the group, indexed by pipeline.
func (g *DriverGroup) Drivers() []*Driver { return g.drivers }

// In is unused: foldspace consumes *message.Message, not *message.Payload.
func (g *DriverGroup) In() chan *message.Payload { return nil }

// PipelineMonitor returns the monitor shared by every driver in the group.
func (g *DriverGroup) PipelineMonitor() metrics.PipelineMonitor { return g.monitor }

// Start starts every driver.
func (g *DriverGroup) Start() {
	for _, d := range g.drivers {
		d.Start()
	}
}

// Stop stops the drivers concurrently, so draining N of them costs one shutdown
// timeout rather than N.
func (g *DriverGroup) Stop() {
	var wg sync.WaitGroup
	for _, d := range g.drivers {
		wg.Add(1)
		go func(d *Driver) {
			defer wg.Done()
			d.Stop()
		}(d)
	}
	wg.Wait()
}

func (d *Driver) ingestLoop() {
	defer close(d.ingestDone)
	defer d.wg.Done()

	// The core seals a batch on record count and content size, so a partial batch
	// left by a lull has nothing to complete it. Seal on a timer as well, matching
	// the primary destination's batch strategy. Flush belongs to the ingest
	// region, so this goroutine is the only one permitted to call it.
	ticker := time.NewTicker(d.batchWait)
	defer ticker.Stop()

	for {
		select {
		case item := <-d.input:
			d.offer(item)
		case <-ticker.C:
			// A refused flush needs no handling here: the next tick retries it.
			_, progress := d.core.Flush()
			d.dispatch(progress)
		case <-d.ingestStopping:
			// d.input is never closed: Offer/Tap race this same signal on their
			// send, and closing a channel out from under a blocked sender panics
			// it. Drain whatever is already buffered non-blockingly instead.
			for {
				select {
				case item := <-d.input:
					d.offer(item)
				default:
					return
				}
			}
		}
	}
}

func (d *Driver) offer(item ingestItem) {
	for {
		if !d.core.HasCapacity() {
			select {
			case <-d.capacity:
			case <-d.ingestCancel:
				return
			case <-d.stop:
				return
			}
			continue
		}
		id := d.nextID.Add(1)
		if item.meta != nil {
			d.pending.store(id, item.meta)
		}
		now := uint64(time.Now().UnixNano() - d.startNanos)
		route := d.nonMRFRoute
		if d.mrfRoute != 0 && item.record.MRFAllowed && d.mrfEnabled() {
			route |= d.mrfRoute
		}
		admission, progress := d.core.PushLog(item.record, now, id, route)
		d.dispatch(progress)
		switch admission {
		case Accepted:
			return
		case TooLarge:
			d.ackTooLarge(id)
			return
		case Refused:
			d.pending.take(id)
			select {
			case <-d.capacity:
			case <-d.ingestCancel:
				return
			case <-d.stop:
				return
			}
		case ShuttingDown:
			d.pending.take(id)
			return
		}
	}
}

// mrfEnabled reports whether MRF failover is currently active, mirroring
// destination_sender.go's canSend() gate for the primary HTTP path.
func (d *Driver) mrfEnabled() bool {
	return d.cfg != nil &&
		d.cfg.GetBool("multi_region_failover.enabled") &&
		d.cfg.GetBool("multi_region_failover.failover_logs")
}

func (d *Driver) ackTooLarge(id uint64) {
	meta := d.pending.take(id)
	if meta == nil || d.dualShip || d.sink == nil || d.sink.Channel() == nil {
		return
	}
	d.sink.Channel() <- message.NewPayload([]*message.MessageMetadata{meta}, nil, "", 0)
}

func (d *Driver) senderLoop(sender SenderID) {
	defer d.wg.Done()

	// Unacked sends are bounded by the core's max_inflight_payloads, which refuses
	// admission past it, so there is no second bound here. One would have to be at
	// least as large to be harmless and could only deadlock if it were smaller:
	// sends happen on this goroutine, and it is also the only consumer of acks, so
	// blocking a send to wait for an ack waits for something this goroutine is the
	// one responsible for delivering.
	acks := make(chan streamAck, d.pipelineDepth*2)
	timers := make(chan scheduledTimer, 4)

	// stopCtx parents every OpenStream and Stream.Send: an intake that never
	// answers a dial, or stalls a send via gRPC flow control, would otherwise
	// hold this goroutine past d.stop being closed, which is what Stop's
	// d.wg.Wait() waits on. Tying it to d.stop lets shutdown end either without
	// waiting out its timeout.
	stopCtx, cancelStop := context.WithCancel(context.Background())
	defer cancelStop()
	go func() {
		select {
		case <-d.stop:
			cancelStop()
		case <-stopCtx.Done():
		}
	}()

	var current Stream
	var currentID StreamID
	recvCancel := func() {}
	var recvDone chan struct{}

	// The ack watchdog fails a stream holding unacked batches once a whole
	// ackTimeout passes with no ack arriving. It runs on this goroutine rather
	// than as a core timer so that a healthy stream costs one local timer per
	// ackTimeout and only a stalled one crosses into the core. It cannot fire
	// while Send is blocked, which the send deadline bounds.
	var outstanding int // batches sent on current and not yet acked
	var ackCount, ackCountAtArm uint64
	watchdog := time.NewTimer(d.ackTimeout)
	watchdog.Stop()
	defer watchdog.Stop()
	watchdogArmed := false

	stopRecv := func() {
		if current != nil {
			_ = current.Close()
			current = nil
		}
		recvCancel()
		if recvDone != nil {
			<-recvDone
		}
		recvDone = nil
		outstanding = 0
		watchdog.Stop()
		watchdogArmed = false
	}
	defer stopRecv()

	handleAck := func(a streamAck) {
		if a.err != nil {
			progress := d.core.HandleStreamError(sender, a.stream, a.err.Error())
			d.dispatch(progress)
			stopRecv()
			return
		}
		if current != nil && a.stream == currentID && outstanding > 0 {
			outstanding--
			ackCount++
		}
		progress := d.core.HandleAck(sender, a.stream, a.id, a.status)
		d.dispatch(progress)
	}

	drainEffects := func() {
		for {
			effects := d.core.PollSender(sender)
			if len(effects) == 0 {
				return
			}
			for _, effect := range effects {
				switch effect.Kind {
				case OpenStream:
					stopRecv()
					if effect.After > 0 {
						timer := time.NewTimer(effect.After)
						select {
						case <-timer.C:
						case <-d.stop:
							timer.Stop()
							return
						}
					}
					ctx, cancel := context.WithTimeout(stopCtx, d.connectTimeout)
					stream, err := d.transport.OpenStream(ctx, sender, effect.Stream)
					cancel()
					if err != nil {
						progress := d.core.HandleStreamError(sender, effect.Stream, err.Error())
						d.dispatch(progress)
						continue
					}
					progress := d.core.HandleStreamOpened(sender, effect.Stream)
					d.dispatch(progress)
					current = stream
					currentID = effect.Stream
					ctxRecv, cancelRecv := context.WithCancel(context.Background())
					recvCancel = cancelRecv
					recvDone = make(chan struct{})
					go d.recvLoop(ctxRecv, currentID, stream, acks, recvDone)
				case SendBatch:
					if current == nil || currentID != effect.Stream {
						if effect.Batch != nil {
							effect.Batch.Release()
						}
						continue
					}
					data := append([]byte(nil), effect.Batch.Bytes()...)
					effect.Batch.Release()
					ctx, cancel := context.WithTimeout(stopCtx, d.sendTimeout)
					err := current.Send(ctx, effect.BatchID, data)
					cancel()
					if err != nil {
						progress := d.core.HandleStreamError(sender, effect.Stream, err.Error())
						d.dispatch(progress)
						stopRecv()
						continue
					}
					outstanding++
					if !watchdogArmed {
						ackCountAtArm = ackCount
						watchdog.Reset(d.ackTimeout)
						watchdogArmed = true
					}
				case CloseStream:
					stopRecv()
				case ScheduleTimer:
					stream := effect.Stream
					kind := effect.Timer
					time.AfterFunc(effect.After, func() {
						select {
						case timers <- scheduledTimer{stream: stream, kind: kind}:
							d.nudge(sender)
						case <-d.stop:
						}
					})
				case ReportError:
					if effect.Err != nil {
						log.Warnf("foldspace sender %d: %v", sender, effect.Err)
					}
				}
			}
		}
	}

	for {
		select {
		case <-d.stop:
			return
		case <-d.wake[sender]:
			drainEffects()
		case a := <-acks:
			handleAck(a)
			drainEffects()
		case <-watchdog.C:
			watchdogArmed = false
			// An ack already queued here is progress this goroutine has not
			// counted yet, and failing a healthy stream for it would resend the
			// whole window to an intake that is keeping up.
		queued:
			for {
				select {
				case a := <-acks:
					handleAck(a)
				default:
					break queued
				}
			}
			switch {
			case outstanding == 0:
			case ackCount != ackCountAtArm:
				ackCountAtArm = ackCount
				watchdog.Reset(d.ackTimeout)
				watchdogArmed = true
			default:
				progress := d.core.HandleStreamError(sender, currentID, "no batch acknowledged within "+d.ackTimeout.String())
				d.dispatch(progress)
				stopRecv()
			}
			drainEffects()
		case t := <-timers:
			progress := d.core.HandleTimer(sender, t.stream, t.kind)
			d.dispatch(progress)
			drainEffects()
		}
	}
}

func (d *Driver) nudge(sender SenderID) {
	select {
	case d.wake[sender] <- struct{}{}:
	default:
	}
}

func (d *Driver) recvLoop(ctx context.Context, stream StreamID, s Stream, acks chan streamAck, done chan struct{}) {
	defer close(done)
	for {
		batchID, status, err := s.Recv(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case acks <- streamAck{stream: stream, err: err}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case acks <- streamAck{stream: stream, id: batchID, status: status}:
		case <-ctx.Done():
			return
		}
	}
}

func (d *Driver) notifyLoop() {
	defer d.wg.Done()
	for {
		select {
		case <-d.stop:
			d.drainNotifications()
			return
		case <-d.notifyWake:
			d.drainNotifications()
		}
	}
}

func (d *Driver) drainNotifications() {
	for _, n := range d.core.PollNotifications() {
		d.handleNotification(n)
	}
}

func (d *Driver) handleNotification(n Notification) {
	switch n.Kind {
	case PayloadDurable:
		d.releaseToSink(n.MetadataIDs)
	case PayloadAbandoned:
		d.dropPending(n.MetadataIDs)
	case DroppedStats:
		metrics.TlmFoldspaceDropped.Add(float64(n.Records))
	}
}

func (d *Driver) releaseToSink(ids []uint64) {
	if d.dualShip {
		d.dropPending(ids)
		return
	}
	metas := d.pending.takeAll(ids)
	if len(metas) == 0 || d.sink == nil || d.sink.Channel() == nil {
		return
	}
	d.sink.Channel() <- message.NewPayload(metas, nil, "", 0)
}

func (d *Driver) dropPending(ids []uint64) {
	d.pending.takeAll(ids)
}

func (d *Driver) dispatch(progress Progress) {
	for _, sender := range progress.Wake {
		d.nudge(sender)
	}
	if progress.HasCapacity {
		select {
		case d.capacity <- struct{}{}:
		default:
		}
	}
	if progress.NotificationsReady {
		select {
		case d.notifyWake <- struct{}{}:
		default:
		}
	}
}

type pendingTable struct {
	mu   sync.Mutex
	byID map[uint64]*message.MessageMetadata
}

func newPendingTable() *pendingTable {
	return &pendingTable{byID: make(map[uint64]*message.MessageMetadata)}
}

func (p *pendingTable) store(id uint64, meta *message.MessageMetadata) {
	p.mu.Lock()
	p.byID[id] = meta
	p.mu.Unlock()
}

func (p *pendingTable) take(id uint64) *message.MessageMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	meta := p.byID[id]
	delete(p.byID, id)
	return meta
}

func (p *pendingTable) takeAll(ids []uint64) []*message.MessageMetadata {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*message.MessageMetadata, 0, len(ids))
	for _, id := range ids {
		if meta, ok := p.byID[id]; ok {
			out = append(out, meta)
			delete(p.byID, id)
		}
	}
	return out
}

// FanInStrategy forwards processor output onto a shared Driver.
type FanInStrategy struct {
	in     chan *message.Message
	driver *Driver
	done   chan struct{}
}

// NewFanInStrategy returns a Strategy that offers each message to driver.
func NewFanInStrategy(in chan *message.Message, driver *Driver) *FanInStrategy {
	return &FanInStrategy{in: in, driver: driver, done: make(chan struct{})}
}

// Start pumps messages into the driver.
func (s *FanInStrategy) Start() {
	go func() {
		defer close(s.done)
		for msg := range s.in {
			s.driver.Offer(msg)
		}
	}()
}

// Stop closes the input and waits for the pump to finish.
func (s *FanInStrategy) Stop() {
	close(s.in)
	<-s.done
}

// DriverTap maps each rendered message onto a foldspace Record and hands that to
// the driver's ingest channel.
//
// It must snapshot rather than reference the message, because the primary
// destination's encoder rewrites the message in place immediately afterwards. A
// Record is the whole snapshot: it copies the body and tags out, so the mapping
// costs one copy of the body and nothing else. The metadata stays behind, since
// the primary destination owns the auditor sink.
//
// The hand-off blocks. Foldspace's ingest buffer is sized independently of the
// primary destination's, so an instantaneous rate difference is absorbed there
// and neither destination waits on the other. A buffer that fills anyway means
// the pipeline is running faster than a destination can ship, which is what
// back-pressure is for; shedding instead would trade a delay for a lost log.
//
// It gives up once the driver's Stop has started rather than blocking the
// processor goroutine past that point: see Driver.Offer for why.
type DriverTap struct {
	input    chan ingestItem
	stopping chan struct{}
}

// Tap snapshots msg onto the driver's ingest channel.
func (t *DriverTap) Tap(msg *message.Message) {
	select {
	case t.input <- ingestItem{record: recordFromMessage(msg)}:
	case <-t.stopping:
	}
}
