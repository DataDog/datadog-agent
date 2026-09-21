// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package foldspace drives the sans-I/O foldspace library from the logs agent.
//
// The library owns tokenize/cluster/intern/batch/encode/compress and stream
// lifecycle decisions. This package owns threads, gRPC, timers, and auditor
// release. It is a sender.Strategy, not a client.Destination: it consumes
// processor output as one record per call rather than already-batched JSON.
package foldspace

import (
	"fmt"
	"sync/atomic"
	"time"
)

// SenderID names one endpoint for the life of a client: SenderID(i) is
// Config.Endpoints[i].
type SenderID uint64

// StreamID names one connection generation. The library mints these; a
// consumer carries one back with the event it belongs to so the library can
// tell a live stream from one it has already replaced.
type StreamID uint64

// SenderClass is what a failure means for one endpoint.
type SenderClass uint8

const (
	// Reliable senders replay what they have not had acknowledged, and only
	// their acknowledgement makes a payload durable. At least one sender must
	// be reliable.
	Reliable SenderClass = 0
	// Unreliable senders attempt once, discard what fails, and promise
	// nothing.
	Unreliable SenderClass = 1
)

// Compression is the encoding applied to every batch body.
type Compression int

const (
	// Identity leaves bodies uncompressed.
	Identity Compression = 0
	// Zstd compresses each body at Config.ZstdLevel.
	Zstd Compression = 1
)

// Admission is what the library did with an offered record, or with a flush.
type Admission int

const (
	// Accepted means the record and its metadata id belong to the library.
	Accepted Admission = 1
	// Refused means the retained set is at its bound. Nothing was translated,
	// batched, or sealed, and the same record can be offered again unchanged.
	Refused Admission = 2
	// TooLarge means no representation of the record fits a payload, so it was
	// not taken and offering it again unchanged cannot succeed.
	TooLarge Admission = 3
	// ShuttingDown means admission is closed and the retained set is draining.
	ShuttingDown Admission = 4
)

func (a Admission) String() string {
	switch a {
	case Accepted:
		return "accepted"
	case Refused:
		return "refused"
	case TooLarge:
		return "too large"
	case ShuttingDown:
		return "shutting down"
	default:
		return fmt.Sprintf("admission(%d)", int(a))
	}
}

// EffectKind is what the consumer must do.
type EffectKind int

const (
	// OpenStream asks for a new connection for this sender, after Effect.After.
	OpenStream EffectKind = 1
	// SendBatch asks for these bytes to go out on the stream it names.
	SendBatch EffectKind = 2
	// CloseStream asks for this sender's stream to be closed.
	CloseStream EffectKind = 3
	// ScheduleTimer asks for a timer to be fed back after Effect.After.
	ScheduleTimer EffectKind = 4
	// ReportError carries a protocol error against one sender.
	ReportError EffectKind = 5
)

func (k EffectKind) String() string {
	switch k {
	case OpenStream:
		return "open stream"
	case SendBatch:
		return "send batch"
	case CloseStream:
		return "close stream"
	case ScheduleTimer:
		return "schedule timer"
	case ReportError:
		return "report error"
	default:
		return fmt.Sprintf("effect(%d)", int(k))
	}
}

// TimerKind is which timer to schedule.
type TimerKind int

const (
	// RotateStream ends a stream that has reached its lifetime.
	RotateStream TimerKind = 1
	// DrainExpired gives up waiting for a rotating stream's outstanding acks.
	DrainExpired TimerKind = 2
)

// CoreErrorKind is which protocol error the library reported.
type CoreErrorKind int

const (
	// AckMismatch names an acknowledgement for a batch that was not next.
	AckMismatch CoreErrorKind = 1
	// AckWithoutOutstandingBatch names an acknowledgement with nothing owed.
	AckWithoutOutstandingBatch CoreErrorKind = 2
	// BatchRejected names a batch the server did not accept.
	BatchRejected CoreErrorKind = 3
	// StreamFailed names a stream failure the consumer reported.
	StreamFailed CoreErrorKind = 4
)

// NotificationKind is which outcome a notification reports.
type NotificationKind int

const (
	// PayloadDurable resolves every record it names.
	PayloadDurable NotificationKind = 1
	// PayloadDropped names records a sender gave up on, or that abandonment
	// gave up on globally.
	PayloadDropped NotificationKind = 2
)

// AckOK is the wire status of a batch the server accepted. Anything else
// recycles the stream.
const AckOK int32 = 1

// Endpoint is one destination and the delivery contract wanted from it.
//
// Nothing here crosses into the library except Class: addresses, credentials,
// TLS, flow control, connect bounds, and window depth are the consumer's.
type Endpoint struct {
	Address string
	Class   SenderClass
}

// Config describes one client.
type Config struct {
	Endpoints              []Endpoint
	MaxInflightPayloads    int
	BatchCapacity          int
	MaxPayloadBytes        int
	CoalesceThresholdBytes int
	Compression            Compression
	ZstdLevel              int
	ReconnectBackoffBase   time.Duration
	ReconnectBackoffFactor uint32
	ReconnectBackoffCap    time.Duration
	DrainTimeout           time.Duration
	StreamLifetime         time.Duration
	FirstPayloadBatchID    uint32
	SnapshotBatchID        uint32
}

// Record is one log offered to the library.
//
// An empty string is absent. Nothing here is retained past the call that
// carries it.
type Record struct {
	Body            []byte
	TimestampMillis int64
	Service         string
	Status          string
	Source          string
	Hostname        string
	UUID            string
	Tags            []string
	ProcessingTags  []string
}

// Progress is what a state-changing call left behind. Every field is a level
// rather than an edge, so a consumer acting on one has not had to observe
// every prior call.
type Progress struct {
	Wake               []SenderID
	HasCapacity        bool
	NotificationsReady bool
}

// CoreError is a protocol error the library reported against one sender.
type CoreError struct {
	Kind            CoreErrorKind
	ExpectedBatchID uint32
	ActualBatchID   uint32
	BatchID         uint32
	BatchStatus     int32
	Message         string
}

func (e *CoreError) Error() string {
	if e == nil {
		return "foldspace: nil core error"
	}
	switch e.Kind {
	case AckMismatch:
		return fmt.Sprintf("batch %d acknowledged while %d was owed", e.ActualBatchID, e.ExpectedBatchID)
	case AckWithoutOutstandingBatch:
		return "a batch was acknowledged with none outstanding"
	case BatchRejected:
		return fmt.Sprintf("batch %d rejected with status %d", e.BatchID, e.BatchStatus)
	case StreamFailed:
		return "stream failed: " + e.Message
	default:
		return fmt.Sprintf("core error (%d)", int(e.Kind))
	}
}

// Effect is one thing the consumer must do. Which fields are meaningful
// follows from Kind.
type Effect struct {
	Kind    EffectKind
	Sender  SenderID
	Stream  StreamID
	After   time.Duration
	Timer   TimerKind
	BatchID uint32
	Batch   *Lease
	Err     *CoreError
}

// Notification is a final or diagnostic outcome for records the consumer
// offered.
type Notification struct {
	Kind        NotificationKind
	Sender      SenderID
	HasSender   bool
	Abandoned   bool
	MetadataIDs []uint64
}

// Lease holds the bytes of one sealed batch.
//
// It is independent of the effect that carried it and of the client: a sender
// writing asynchronously outlives both, and every sender carrying one payload
// holds its own lease on the same bytes. Release it exactly once when the
// write is done.
type Lease struct {
	bytes     []byte
	released  atomic.Bool
	onRelease func()
}

// NewLease returns a lease over a copy of data. onRelease runs at most once,
// on the first Release.
func NewLease(data []byte, onRelease func()) *Lease {
	copied := append([]byte(nil), data...)
	return &Lease{bytes: copied, onRelease: onRelease}
}

// Bytes are the batch body, valid until Release. Do not retain the slice past
// it.
func (l *Lease) Bytes() []byte {
	if l == nil {
		return nil
	}
	return l.bytes
}

// Release frees the bytes. Calling it twice is a bug; calling it on nil is
// not, so a consumer can defer it unconditionally.
func (l *Lease) Release() {
	if l == nil {
		return
	}
	if l.released.CompareAndSwap(false, true) && l.onRelease != nil {
		l.onRelease()
	}
	l.bytes = nil
}

// Released reports whether Release has been called.
func (l *Lease) Released() bool {
	if l == nil {
		return true
	}
	return l.released.Load()
}
