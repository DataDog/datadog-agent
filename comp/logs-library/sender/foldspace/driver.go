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
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DriverOptions configures one Driver.
type DriverOptions struct {
	Core              Core
	Transport         Transport
	Sink              sender.Sink
	PipelineMonitor   metrics.PipelineMonitor
	InputSize         int
	PipelineDepth     int
	ConnectTimeout    time.Duration
	ShutdownTimeout   time.Duration
	StateRequestBytes int
	// DualShip, when true, never writes the auditor sink and drops on Refused
	// instead of waiting for capacity.
	DualShip bool
}

// Driver consumes processor output, drives a Core, and fans payloads out
// through Transport. It implements sender.PipelineComponent so the pipeline
// provider can start and stop it with the rest of the logs agent.
type Driver struct {
	core      Core
	transport Transport
	sink      sender.Sink
	monitor   metrics.PipelineMonitor

	input           chan *message.Message
	pipelineDepth   int
	connectTimeout  time.Duration
	shutdownTimeout time.Duration
	dualShip        bool

	pending    *pendingTable
	nextID     atomic.Uint64
	startNanos int64

	wake       []chan struct{}
	notifyWake chan struct{}
	capacity   chan struct{}

	stopOnce     sync.Once
	stop         chan struct{}
	stopped      chan struct{}
	ingestDone   chan struct{}
	ingestCancel chan struct{}

	wg sync.WaitGroup
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
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 15 * time.Second
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
		input:           make(chan *message.Message, opts.InputSize),
		pipelineDepth:   opts.PipelineDepth,
		connectTimeout:  opts.ConnectTimeout,
		shutdownTimeout: opts.ShutdownTimeout,
		dualShip:        opts.DualShip,
		pending:         newPendingTable(),
		startNanos:      time.Now().UnixNano(),
		wake:            wake,
		notifyWake:      make(chan struct{}, 1),
		capacity:        make(chan struct{}, 1),
		stop:            make(chan struct{}),
		stopped:         make(chan struct{}),
		ingestDone:      make(chan struct{}),
		ingestCancel:    make(chan struct{}),
	}
}

// In is unused: foldspace consumes *message.Message, not *message.Payload.
func (d *Driver) In() chan *message.Payload { return nil }

// PipelineMonitor returns the monitor shared with the processor.
func (d *Driver) PipelineMonitor() metrics.PipelineMonitor { return d.monitor }

// Input is the ingest channel processor output fans into.
func (d *Driver) Input() chan *message.Message { return d.input }

// Offer sends msg to ingest. Dual-ship drops rather than blocking when the
// buffer is full. Foldspace-only blocks so back-pressure reaches the processor.
func (d *Driver) Offer(msg *message.Message) {
	if d.dualShip {
		select {
		case d.input <- msg:
		default:
			metrics.TlmFoldspaceDualShipDropped.Inc()
		}
		return
	}
	d.input <- msg
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
		close(d.input)
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

func (d *Driver) ingestLoop() {
	defer close(d.ingestDone)
	defer d.wg.Done()
	for msg := range d.input {
		d.offer(msg)
	}
}

func (d *Driver) offer(msg *message.Message) {
	for {
		if !d.core.HasCapacity() {
			if d.dualShip {
				metrics.TlmFoldspaceDualShipDropped.Inc()
				return
			}
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
		d.pending.store(id, &msg.MessageMetadata)
		now := uint64(time.Now().UnixNano() - d.startNanos)
		admission, progress := d.core.PushLog(recordFromMessage(msg), now, id)
		d.dispatch(progress)
		switch admission {
		case Accepted:
			return
		case TooLarge:
			d.ackTooLarge(id)
			return
		case Refused:
			d.pending.take(id)
			if d.dualShip {
				metrics.TlmFoldspaceDualShipDropped.Inc()
				return
			}
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

func (d *Driver) ackTooLarge(id uint64) {
	meta := d.pending.take(id)
	if meta == nil || d.dualShip || d.sink == nil || d.sink.Channel() == nil {
		return
	}
	d.sink.Channel() <- message.NewPayload([]*message.MessageMetadata{meta}, nil, "", 0)
}

func (d *Driver) senderLoop(sender SenderID) {
	defer d.wg.Done()

	sem := make(chan struct{}, d.pipelineDepth)
	acks := make(chan streamAck, d.pipelineDepth*2)
	timers := make(chan scheduledTimer, 4)

	var current Stream
	var currentID StreamID
	recvCancel := func() {}
	var recvDone chan struct{}

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
	}
	defer stopRecv()

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
					ctx, cancel := context.WithTimeout(context.Background(), d.connectTimeout)
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
					select {
					case sem <- struct{}{}:
					case <-d.stop:
						return
					}
					if err := current.Send(context.Background(), effect.BatchID, data); err != nil {
						<-sem
						progress := d.core.HandleStreamError(sender, effect.Stream, err.Error())
						d.dispatch(progress)
						stopRecv()
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
			if a.err != nil {
				progress := d.core.HandleStreamError(sender, a.stream, a.err.Error())
				d.dispatch(progress)
				stopRecv()
				drainEffects()
				continue
			}
			select {
			case <-sem:
			default:
			}
			progress := d.core.HandleAck(sender, a.stream, a.id, a.status)
			d.dispatch(progress)
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
	case PayloadDropped:
		if n.Abandoned {
			d.dropPending(n.MetadataIDs)
			return
		}
		for range n.MetadataIDs {
			metrics.TlmFoldspaceDropped.Inc()
		}
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

// TeeEncoder clones a rendered message onto tap before running inner Encode.
// Dual-ship uses this so HTTP JSON encode cannot mutate the foldspace copy,
// and a full tap never stalls HTTP.
type TeeEncoder struct {
	inner processor.Encoder
	tap   chan *message.Message
}

// NewTeeEncoder returns an encoder that clones onto tap then encodes with inner.
func NewTeeEncoder(inner processor.Encoder, tap chan *message.Message) *TeeEncoder {
	return &TeeEncoder{inner: inner, tap: tap}
}

// Encode clones msg (rendered) onto tap, dropping if tap is full, then encodes.
func (t *TeeEncoder) Encode(msg *message.Message, hostname string) error {
	clone := cloneMessage(msg)
	select {
	case t.tap <- clone:
	default:
		metrics.TlmFoldspaceDualShipDropped.Inc()
	}
	return t.inner.Encode(msg, hostname)
}
