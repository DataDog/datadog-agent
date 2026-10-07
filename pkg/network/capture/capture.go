// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// capturer is the concrete implementation of the Capturer interface.
type capturer struct {
	cfg CaptureConfig

	// handle is the activated libpcap handle, set on Start.
	handle *pcap.Handle
	// filter re-checks every packet in user space. libpcap starts receiving
	// when the handle is activated, before the kernel filter is installed, so
	// packets read in that window would otherwise bypass the filter.
	filter *pcap.BPF

	// Background goroutine lifecycle.
	stopCh chan struct{}
	doneCh chan struct{}
	// drainStarted is set to true just before the drain goroutine is launched
	// so that Stop() knows whether to wait on doneCh.
	drainStarted atomic.Bool

	// Statistics — all updated atomically.
	packetsCaptured  atomic.Uint64
	packetsDropped   atomic.Uint64
	bytesCaptured    atomic.Uint64
	headersTruncated atomic.Uint64
	errCount         atomic.Uint64

	// startTime / endTime protected by mu.
	mu        sync.Mutex
	startTime time.Time
	endTime   time.Time

	// Guards against double-Start / double-Stop.
	started atomic.Bool
	stopped atomic.Bool
}

// newCapturer validates cfg, applies defaults, and checks the BPF filter
// syntax. It does not open a capture.
func newCapturer(cfg CaptureConfig) (*capturer, error) {
	if cfg.Output == nil {
		return nil, errors.New("capture: Output must not be nil")
	}

	cfg.applyDefaults()

	// The link type is only known once the handle is activated; Start compiles
	// the filter again against it. This only rejects bad syntax early.
	if cfg.Filter != "" {
		if _, err := pcap.NewBPF(layers.LinkTypeLinuxSLL, int(cfg.SnapLen), cfg.Filter); err != nil {
			return nil, fmt.Errorf("capture: compiling BPF filter %q: %w", cfg.Filter, err)
		}
	}

	return &capturer{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}, nil
}

// Start implements Capturer. It opens and activates the libpcap handle,
// installs the filter, writes the PCAP global header, and launches the drain
// goroutine.
func (c *capturer) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return errors.New("capture: already started")
	}

	c.mu.Lock()
	c.startTime = time.Now()
	c.mu.Unlock()

	handle, err := c.openHandle()
	if err != nil {
		// The drain goroutine never starts, so Stop has nothing to wait for.
		c.stopped.Store(true)
		return fmt.Errorf("capture: %w", err)
	}
	c.handle = handle

	if err := writePCAPHeader(c.cfg.Output, c.cfg.SnapLen, handle.LinkType()); err != nil {
		c.stopped.Store(true)
		handle.Close()
		return fmt.Errorf("capture: writing PCAP header: %w", err)
	}

	c.drainStarted.Store(true)
	go c.drainLoop(ctx)

	return nil
}

// openHandle activates a libpcap handle on the configured interface. The snap
// length is set before activation, so it applies from the first packet.
func (c *capturer) openHandle() (*pcap.Handle, error) {
	inactive, err := pcap.NewInactiveHandle(c.cfg.Interface)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", c.cfg.Interface, err)
	}
	defer inactive.CleanUp()

	settings := []func() error{
		func() error { return inactive.SetSnapLen(int(c.cfg.SnapLen)) },
		func() error { return inactive.SetPromisc(false) },
		func() error { return inactive.SetTimeout(readTimeout) },
		func() error { return inactive.SetBufferSize(defaultBufferSize) },
	}
	for _, set := range settings {
		if err := set(); err != nil {
			return nil, fmt.Errorf("configuring %s: %w", c.cfg.Interface, err)
		}
	}

	handle, err := inactive.Activate()
	if err != nil {
		return nil, fmt.Errorf("activating %s: %w", c.cfg.Interface, err)
	}

	if c.cfg.Filter != "" {
		if err := handle.SetBPFFilter(c.cfg.Filter); err != nil {
			handle.Close()
			return nil, fmt.Errorf("installing BPF filter %q: %w", c.cfg.Filter, err)
		}
		if c.filter, err = handle.NewBPF(c.cfg.Filter); err != nil {
			handle.Close()
			return nil, fmt.Errorf("compiling BPF filter %q: %w", c.cfg.Filter, err)
		}
	}
	return handle, nil
}

// Stop implements Capturer. It signals the drain goroutine, waits for it to
// exit, records libpcap's drop counter, and closes the handle.
func (c *capturer) Stop() error {
	if !c.stopped.CompareAndSwap(false, true) {
		return nil
	}

	close(c.stopCh)

	// The drain loop wakes at least every readTimeout to observe stopCh.
	if c.drainStarted.Load() {
		<-c.doneCh
	}

	var statsErr error
	if c.handle != nil {
		var stats *pcap.Stats
		if stats, statsErr = c.handle.Stats(); statsErr == nil {
			c.packetsDropped.Store(uint64(stats.PacketsDropped))
		}
		c.handle.Close()
	}

	c.mu.Lock()
	c.endTime = time.Now()
	c.mu.Unlock()

	return statsErr
}

// Stats implements Capturer.
func (c *capturer) Stats() CaptureStats {
	c.mu.Lock()
	start := c.startTime
	end := c.endTime
	c.mu.Unlock()

	return CaptureStats{
		PacketsCaptured:  c.packetsCaptured.Load(),
		PacketsDropped:   c.packetsDropped.Load(),
		BytesCaptured:    c.bytesCaptured.Load(),
		HeadersTruncated: c.headersTruncated.Load(),
		StartTime:        start,
		EndTime:          end,
		Errors:           c.errCount.Load(),
	}
}

// drainLoop is the background goroutine that reads packets from libpcap,
// trims them to their headers, and writes PCAP records. Terminates on: stop
// signal, context cancellation, handle EOF, MaxPackets limit, MaxBytes limit,
// or Duration expiry.
func (c *capturer) drainLoop(ctx context.Context) {
	defer close(c.doneCh)

	var deadline <-chan time.Time
	if c.cfg.Duration > 0 {
		t := time.NewTimer(c.cfg.Duration)
		defer t.Stop()
		deadline = t.C
	}

	linkType := c.handle.LinkType()

	// Tracks the true on-disk file size for the MaxBytes cap. Start()
	// has already written the global header by the time we get here.
	fileBytes := pcapFileHeaderSize

	for {
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		case <-deadline:
			return
		default:
		}

		data, ci, err := c.handle.ReadPacketData()
		if errors.Is(err, pcap.NextErrorTimeoutExpired) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			c.errCount.Add(1)
			continue
		}

		if c.filter != nil && !c.filter.Matches(ci, data) {
			continue
		}

		n, failed := headerLen(data, linkType)
		if failed && ci.CaptureLength < ci.Length {
			c.headersTruncated.Add(1)
		}
		pkt := RawPacket{Timestamp: ci.Timestamp, Data: data[:n], OrigLen: uint32(ci.Length)}

		// MaxBytes is checked before the write, not after, so the cap is never
		// exceeded and the file never ends on a half-written record. Dropping
		// this packet ends the capture — a later, smaller packet must not be
		// allowed to slip in under the cap, because that would reorder the
		// capture relative to the wire and silently misrepresent what happened.
		recordSize := pcapPacketHeaderSize + uint64(len(pkt.Data))
		if c.cfg.MaxBytes > 0 && fileBytes+recordSize > c.cfg.MaxBytes {
			return
		}

		if err := writePCAPPacket(c.cfg.Output, pkt); err != nil {
			c.errCount.Add(1)
			continue
		}

		fileBytes += recordSize
		c.packetsCaptured.Add(1)
		c.bytesCaptured.Add(uint64(len(pkt.Data)))

		if c.cfg.MaxPackets > 0 && c.packetsCaptured.Load() >= c.cfg.MaxPackets {
			return
		}
	}
}
