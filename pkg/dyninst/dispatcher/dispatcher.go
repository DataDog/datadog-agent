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

	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Sink is an interface that abstracts the sink for the Actuator.
type Sink interface {
	// HandleEvent is called when an event is received from the kernel.
	//
	// Note that the event must not be referenced after this call returns;
	// the underlying memory is reused. If any of the data is needed after
	// this call, it must be copied.
	HandleEvent(output.Event) error

	// Close will be called when the sink is no longer needed.
	Close()
}

// Dispatcher represents the data plane for a set of eBPF programs.
//
// It reads events from the ringbuffer and dispatches them to the relevant
// sinks.
type Dispatcher struct {
	reader       *ringbuf.Reader
	wg           sync.WaitGroup
	shuttingDown chan<- struct{}

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
func NewDispatcher(reader *ringbuf.Reader) *Dispatcher {
	shuttingDown := make(chan struct{})
	rt := &Dispatcher{
		reader:       reader,
		shuttingDown: shuttingDown,
	}
	rt.mu.sinks = make(map[ir.ProgramID]Sink)
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		_ = rt.run(shuttingDown)
	}()

	return rt
}

// Shutdown shuts down the dispatcher. It returns any errors that occurred while
// from closing the underlying ringbuf.Reader.
func (d *Dispatcher) Shutdown() error {
	close(d.shuttingDown)
	err := d.reader.Close()
	d.wg.Wait()
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
func (d *Dispatcher) UnregisterSink(progID ir.ProgramID) {
	s := func() Sink {
		d.mu.Lock()
		defer d.mu.Unlock()
		s, ok := d.mu.sinks[progID]
		if !ok {
			return nil
		}
		delete(d.mu.sinks, progID)
		return s
	}()
	// TODO: We should flush the reading goroutine to prove that the sink is no
	// longer in use prior to calling Close.
	if s != nil {
		s.Close()
	}
}

// run runs in a separate goroutine and processes messages from the
// ringbuffer and to hand them to the dispatcher.
func (d *Dispatcher) run(shuttingDown <-chan struct{}) (retErr error) {
	reader := d.reader
	inShutdown := func() bool {
		select {
		case <-shuttingDown:
			return true
		default:
			return false
		}
	}
	for {
		if inShutdown() {
			return nil
		}
		rec := recordPool.Get().(*ringbuf.Record)
		if err := reader.ReadInto(rec); err != nil {
			if errors.Is(err, ringbuf.ErrFlushed) {
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
	defer rec.Release()

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
	if err := sink.HandleEvent(ev); err != nil {
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
