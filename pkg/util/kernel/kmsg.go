// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package kernel

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"golang.org/x/sys/unix"
)

const (
	kmsgPath               = "/dev/kmsg"
	defaultKmsgChannelSize = 128
	maxKmsgRecordSize      = 16 * 1024
)

// KmsgRecord is a single record read from /dev/kmsg.
type KmsgRecord struct {
	Facility  uint8
	Priority  uint8
	Sequence  uint64
	Timestamp uint64
	Flags     string
	Message   string
}

// KmsgFilter determines whether a parsed kmsg record is delivered to the reader's output channel.
// Filters run synchronously on the reader goroutine and must be fast, non-blocking, pure, non-reentrant, and non-panicking.
type KmsgFilter func(KmsgRecord) bool

// KmsgReader reads records from /dev/kmsg in a background goroutine.
type KmsgReader struct {
	source kmsgSource

	subscribersMutex sync.RWMutex
	subscribers      map[string]*kmsgSubscriber
	channelSize      int
	stopped          bool

	errors chan error
	stop   chan struct{}
	done   chan struct{}

	stopOnce  sync.Once
	telemetry *kmsgTelemetry
}

type kmsgSubscriber struct {
	name      string
	filter    KmsgFilter
	records   chan KmsgRecord
	delivered telemetry.SimpleCounter
	losses    telemetry.SimpleCounter
}

type kmsgSource interface {
	io.ReadSeeker
	io.Closer
}

type kmsgTelemetry struct {
	once             sync.Once
	read             telemetry.Counter
	delivered        telemetry.Counter
	errors           telemetry.Counter
	losses           telemetry.Counter
	ringBufferLosses telemetry.Counter
}

var kmsgTelemetryDefinitions kmsgTelemetry

func (t *kmsgTelemetry) init(component telemetry.Component) {
	const subsystem = "kernel__kmsg"

	t.once.Do(func() {
		t.read = component.NewCounter(subsystem, "records_read", nil, "Number of records read from /dev/kmsg")
		t.delivered = component.NewCounter(subsystem, "records_delivered", []string{"subscriber"}, "Number of /dev/kmsg records delivered to a subscriber")
		t.errors = component.NewCounter(subsystem, "errors", nil, "Number of /dev/kmsg reader errors")
		t.losses = component.NewCounter(subsystem, "losses", []string{"subscriber"}, "Number of /dev/kmsg records lost because a subscriber channel was full")
		t.ringBufferLosses = component.NewCounter(subsystem, "ring_buffer_losses", nil, "Number of /dev/kmsg records lost because the ring buffer was full")
	})
}

// NewKmsgReader opens /dev/kmsg, seeks to the end of its current buffer, and starts reading future records.
// Telemetry is process-wide: the component passed to the first call owns the metrics for all reader instances.
func NewKmsgReader(component telemetry.Component) (*KmsgReader, error) {
	if component == nil {
		return nil, errors.New("kmsg telemetry component is nil")
	}

	source, err := openKmsg()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", kmsgPath, err)
	}

	kmsgTelemetryDefinitions.init(component)
	reader, err := newKmsgReader(source, &kmsgTelemetryDefinitions, defaultKmsgChannelSize)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	return reader, nil
}

// newKmsgReader creates a reader around a source. Tests use it to substitute /dev/kmsg with a fake source.
func newKmsgReader(source kmsgSource, telemetry *kmsgTelemetry, channelSize int) (*KmsgReader, error) {
	if source == nil {
		return nil, errors.New("kmsg source is nil")
	}
	if telemetry == nil {
		return nil, errors.New("kmsg telemetry is nil")
	}
	if channelSize <= 0 {
		return nil, fmt.Errorf("kmsg channel size must be positive, got %d", channelSize)
	}
	if _, err := source.Seek(0, io.SeekEnd); err != nil {
		return nil, fmt.Errorf("seek kmsg to end: %w", err)
	}

	reader := &KmsgReader{
		source:      source,
		subscribers: make(map[string]*kmsgSubscriber),
		channelSize: channelSize,
		errors:      make(chan error, 1),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		telemetry:   telemetry,
	}
	go reader.run()
	return reader, nil
}

// Subscribe returns a channel that receives future records accepted by filter and a function that unsubscribes it.
// The name identifies the subscriber in telemetry and should be a stable value.
// A nil filter accepts every record.
func (r *KmsgReader) Subscribe(name string, filter KmsgFilter) (<-chan KmsgRecord, func(), error) {
	if name == "" {
		return nil, nil, errors.New("kmsg subscriber name is empty")
	}

	r.subscribersMutex.Lock()
	defer r.subscribersMutex.Unlock()

	if r.stopped {
		return nil, nil, errors.New("kmsg reader is stopped")
	}
	if _, exists := r.subscribers[name]; exists {
		return nil, nil, fmt.Errorf("kmsg subscriber %q already exists", name)
	}

	records := make(chan KmsgRecord, r.channelSize)
	subscriber := &kmsgSubscriber{
		name:      name,
		filter:    filter,
		records:   records,
		delivered: r.telemetry.delivered.WithTags(map[string]string{"subscriber": name}),
		losses:    r.telemetry.losses.WithTags(map[string]string{"subscriber": name}),
	}
	r.subscribers[name] = subscriber
	return records, func() { r.unsubscribe(subscriber) }, nil
}

func (r *KmsgReader) unsubscribe(subscriber *kmsgSubscriber) {
	r.subscribersMutex.Lock()
	defer r.subscribersMutex.Unlock()

	r.unsubscribeLocked(subscriber)
}

func (r *KmsgReader) unsubscribeLocked(subscriber *kmsgSubscriber) {
	current, exists := r.subscribers[subscriber.name]
	if !exists || current != subscriber {
		return
	}
	delete(r.subscribers, subscriber.name)
	close(subscriber.records)
}

// Errors returns a channel that receives at most one terminal reader error.
func (r *KmsgReader) Errors() <-chan error {
	return r.errors
}

// Stop terminates the reader and closes its channels. It is safe to call multiple times.
func (r *KmsgReader) Stop() {
	r.stopOnce.Do(func() {
		close(r.stop)
		_ = r.source.Close()
		<-r.done
	})
}

func (r *KmsgReader) run() {
	defer func() {
		r.subscribersMutex.Lock()
		r.stopped = true
		for _, subscriber := range r.subscribers {
			r.unsubscribeLocked(subscriber)
		}
		r.subscribersMutex.Unlock()

		close(r.errors)
		close(r.done)
	}()

	buffer := make([]byte, maxKmsgRecordSize)

	for {
		n, err := r.source.Read(buffer)
		if err != nil {
			if r.stopping() {
				return
			}
			if errors.Is(err, unix.EPIPE) {
				r.telemetry.ringBufferLosses.Inc()
				// EPIPE means that /dev/kmsg's ring buffer discarded unread records.
				continue
			}
			r.telemetry.errors.Inc()
			r.reportTerminalError(fmt.Errorf("read kmsg record: %w", err))
			return
		}
		if n == 0 {
			r.telemetry.errors.Inc()
			r.reportTerminalError(errors.New("read kmsg record returned no data"))
			return
		}
		r.telemetry.read.Inc()

		record, err := parseKmsgRecord(buffer[:n])
		if err != nil {
			r.telemetry.errors.Inc()
			continue
		}

		r.subscribersMutex.RLock()
		for _, subscriber := range r.subscribers {
			if subscriber.filter != nil && !subscriber.filter(record) {
				continue
			}

			select {
			case subscriber.records <- record:
				subscriber.delivered.Inc()
			default:
				subscriber.losses.Inc()
			}
		}
		r.subscribersMutex.RUnlock()
	}
}

func (r *KmsgReader) stopping() bool {
	select {
	case <-r.stop:
		return true
	default:
		return false
	}
}

func (r *KmsgReader) reportTerminalError(err error) {
	select {
	case r.errors <- err:
	default:
	}
}

func parseKmsgRecord(raw []byte) (KmsgRecord, error) {
	line := strings.TrimSuffix(string(raw), "\n")
	header, message, found := strings.Cut(line, ";")
	if !found {
		return KmsgRecord{}, errors.New("missing kmsg header separator")
	}

	fields := strings.Split(header, ",")
	if len(fields) < 4 {
		return KmsgRecord{}, fmt.Errorf("expected at least 4 kmsg header fields, got %d", len(fields))
	}

	pri, err := strconv.ParseUint(fields[0], 10, 8)
	if err != nil {
		return KmsgRecord{}, fmt.Errorf("parse kmsg priority: %w", err)
	}
	sequence, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return KmsgRecord{}, fmt.Errorf("parse kmsg sequence: %w", err)
	}
	timestamp, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return KmsgRecord{}, fmt.Errorf("parse kmsg timestamp: %w", err)
	}

	return KmsgRecord{
		Facility:  uint8(pri >> 3),
		Priority:  uint8(pri & 0x7),
		Sequence:  sequence,
		Timestamp: timestamp,
		Flags:     fields[3],
		Message:   message,
	}, nil
}

// openKmsg creates a runtime-pollable /dev/kmsg source.
//
// O_CLOEXEC prevents the descriptor from leaking into child processes. O_NONBLOCK lets os.NewFile register the
// descriptor with Go's runtime poller, so Read waits for records without blocking an OS thread and Close cancels an
// in-progress Read. Do not call File.Fd on the returned file: on Unix, doing so disables its deadline and poller support.
func openKmsg() (*os.File, error) {
	fd, err := unix.Open(kmsgPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), kmsgPath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create kmsg file")
	}
	return file, nil
}
