// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package dispatcher is responsible for forwarding data from the ebpf
// ringbuffer to per-program sinks.
package dispatcher

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Sink is an interface that abstracts the sink for the Actuator.
type Sink interface {
	// HandleEvent is called when an message is received from the kernel.
	//
	// Note that the caller may release the Message via its Release method for
	// memory reuse.
	HandleEvent(Message) error

	// HandleDropNotification is called when a side-channel drop notification
	// is received. Notifications are fire-and-forget from the dispatcher's
	// perspective; the sink is responsible for any serialization it needs
	// relative to HandleEvent.
	HandleDropNotification(output.DropNotification)

	// EvictOlderThan asks the sink to finalize any buffered entries whose
	// invocation predates cutoffKtimeNs (in the bpf_ktime_get_ns domain).
	// Called from the actuator goroutine when BPF reported that a drop
	// notification was itself lost and a grace window has elapsed. The
	// sink applies its own monotonic check, so callers may invoke this
	// freely with the latest cutoff on each stats poll.
	EvictOlderThan(cutoffKtimeNs uint64)

	// Close will be called when the sink is no longer needed.
	Close()
}

// Dispatcher represents the data plane for a set of eBPF programs.
//
// It reads events from the ringbuffer and dispatches them to the relevant
// sinks.
type Dispatcher struct {
	reader           *ringbuf.Reader
	dropNotifyReader *ringbuf.Reader // may be nil
	wg               sync.WaitGroup
	shuttingDown     chan<- struct{}

	flush struct {
		*sync.Cond      // uses its own sync.Mutex
		flushing   bool // true while a flush cycle is in progress
		stopped    bool // true after the run goroutine exits
	}

	mu struct {
		sync.Mutex
		sinks map[ir.ProgramID]Sink
	}
}

// NewDispatcher creates a new dispatcher.
//
// The dispatcher must be shutdown to avoid leaking resources. Note that from
// this point forth, the dispatcher owns the reader; shutting down the
// dispatcher will close the reader.
//
// dropNotifyReader is optional. If non-nil, the dispatcher spawns a second
// reader goroutine that reads fixed-size drop notifications from that reader
// and delivers them to sinks via HandleDropNotification.
func NewDispatcher(reader, dropNotifyReader *ringbuf.Reader) *Dispatcher {
	shuttingDown := make(chan struct{})
	rt := &Dispatcher{
		reader:           reader,
		dropNotifyReader: dropNotifyReader,
		shuttingDown:     shuttingDown,
	}
	rt.flush.Cond = sync.NewCond(&sync.Mutex{})
	rt.mu.sinks = make(map[ir.ProgramID]Sink)
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		_ = rt.run(shuttingDown)
	}()
	if dropNotifyReader != nil {
		rt.wg.Add(1)
		go func() {
			defer rt.wg.Done()
			rt.runDropNotify(shuttingDown)
		}()
	}

	return rt
}

// Shutdown shuts down the dispatcher. It returns any errors that occurred while
// from closing the underlying ringbuf.Reader.
func (d *Dispatcher) Shutdown() error {
	d.flushAndWait()

	close(d.shuttingDown)
	err := d.reader.Close()
	if d.dropNotifyReader != nil {
		err = errors.Join(err, d.dropNotifyReader.Close())
	}
	d.wg.Wait()

	// Close any remaining sinks.
	d.mu.Lock()
	sinks := d.mu.sinks
	d.mu.sinks = nil
	d.mu.Unlock()
	for _, s := range sinks {
		s.Close()
	}

	return err
}

// RegisterSink registers a sink for a program.
//
// The sink will receive events for the program.
func (d *Dispatcher) RegisterSink(progID ir.ProgramID, sink Sink) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mu.sinks[progID] = sink
}

// UnregisterSink will unregister the sink associated with the program if one
// has been registered.
//
// The sink will no longer receive events for the program. If the sink is not
// registered, this is a no-op. If it is, the Close method will be called.
//
// This method flushes the ringbuffer before removing and closing the sink,
// ensuring that all pending events are delivered and no concurrent
// HandleEvent call is in progress when Close is called.
func (d *Dispatcher) UnregisterSink(progID ir.ProgramID) {
	d.mu.Lock()
	_, registered := d.mu.sinks[progID]
	d.mu.Unlock()
	if !registered {
		return
	}

	d.flushAndWait()

	s := func() Sink {
		d.mu.Lock()
		defer d.mu.Unlock()
		s := d.mu.sinks[progID]
		delete(d.mu.sinks, progID)
		return s
	}()
	if s != nil {
		s.Close()
	}
}

// EvictOlderThan forwards an eviction request to the sink registered for
// progID, if any. Returns without error if the sink is not registered —
// the program may have been unregistered concurrently.
func (d *Dispatcher) EvictOlderThan(progID ir.ProgramID, cutoffKtimeNs uint64) {
	sink, ok := d.getSink(progID)
	if !ok {
		return
	}
	sink.EvictOlderThan(cutoffKtimeNs)
}

// flushAndWait triggers a flush of the ringbuffer reader and waits until the
// run goroutine has processed all pending records and acknowledged the flush.
// Concurrent callers are serialized: only one flush cycle is in flight at a
// time.
func (d *Dispatcher) flushAndWait() {
	d.flush.L.Lock()
	defer d.flush.L.Unlock()

	// Wait until no other flush is in progress or the run goroutine has
	// stopped.
	for d.flush.flushing && !d.flush.stopped {
		d.flush.Wait()
	}
	if d.flush.stopped {
		return
	}

	// Begin our flush cycle.
	d.flush.flushing = true
	d.flush.L.Unlock()
	d.reader.Flush()
	d.flush.L.Lock()

	// Wait until the run goroutine acks our flush (sets flushing=false) or
	// the run goroutine has stopped.
	for d.flush.flushing && !d.flush.stopped {
		d.flush.Wait()
	}
}

// run runs in a separate goroutine and processes messages from the
// ringbuffer and to hand them to the dispatcher.
func (d *Dispatcher) run(shuttingDown <-chan struct{}) (retErr error) {
	defer func() {
		d.flush.L.Lock()
		d.flush.stopped = true
		d.flush.Broadcast()
		d.flush.L.Unlock()
	}()

	reader := d.reader
	inShutdown := func() bool {
		select {
		case <-shuttingDown:
			return true
		default:
			return false
		}
	}
	ackFlush := func() {
		d.flush.L.Lock()
		d.flush.flushing = false
		d.flush.Broadcast()
		d.flush.L.Unlock()
	}
	for {
		if inShutdown() {
			return nil
		}
		rec := recordPool.Get().(*ringbuf.Record)
		if err := reader.ReadInto(rec); err != nil {
			if errors.Is(err, ringbuf.ErrFlushed) {
				ackFlush()
				continue
			}
			return fmt.Errorf("error reading message: %w", err)
		}

		// TODO: Improve error handling here.
		//
		// Perhaps we want to find a way to only partially fail. Alternatively,
		// this interface should not be delivering errors at all.
		if err := d.handleMessage(Message{
			rec: rec,
		}); err != nil && !inShutdown() {
			log.Errorf("error handling message: %v", err)
			return fmt.Errorf("error handling message: %w", err)
		}
	}
}

func (d *Dispatcher) handleMessage(rec Message) error {
	ev := rec.Event()
	evHeader, err := ev.Header()
	if err != nil {
		return fmt.Errorf("error getting event header: %w", err)
	}

	progID := ir.ProgramID(evHeader.Prog_id)
	sink, ok := d.getSink(progID)
	if !ok {
		return fmt.Errorf("no sink for program %d", progID)
	}
	if err := sink.HandleEvent(rec); err != nil {
		return fmt.Errorf("error handling event: %w", err)
	}
	return nil
}

func (d *Dispatcher) getSink(progID ir.ProgramID) (Sink, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	sink, ok := d.mu.sinks[progID]
	return sink, ok
}

// dropNotifySize is the exact byte size of one drop notification. The
// side-channel ringbuf carries nothing else, so every record has this size.
const dropNotifySize = int(unsafe.Sizeof(output.DropNotification{}))

// runDropNotify drains the side-channel ringbuf, decoding each record as a
// DropNotification and routing it to the sink identified by Prog_id. Shares
// the same shutdown channel as the main reader so Shutdown closes both.
func (d *Dispatcher) runDropNotify(shuttingDown <-chan struct{}) {
	reader := d.dropNotifyReader
	inShutdown := func() bool {
		select {
		case <-shuttingDown:
			return true
		default:
			return false
		}
	}
	var rec ringbuf.Record
	for {
		if inShutdown() {
			return
		}
		if err := reader.ReadInto(&rec); err != nil {
			if errors.Is(err, ringbuf.ErrClosed) || inShutdown() {
				return
			}
			// ErrFlushed is not used on this reader — no flush path. Any
			// other error aborts the goroutine with a log.
			log.Errorf("drop-notify reader: %v", err)
			return
		}
		if len(rec.RawSample) != dropNotifySize {
			log.Errorf(
				"drop-notify: unexpected record size %d, want %d",
				len(rec.RawSample), dropNotifySize,
			)
			continue
		}
		notif := *(*output.DropNotification)(unsafe.Pointer(&rec.RawSample[0]))
		progID := ir.ProgramID(notif.Prog_id)
		sink, ok := d.getSink(progID)
		if !ok {
			// Sink was unregistered between BPF emitting the notification
			// and us reading it. Nothing to do.
			continue
		}
		sink.HandleDropNotification(notif)
	}
}
