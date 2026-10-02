// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build foldspace

package foldspace

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/nativelib/linux_amd64 -lfoldspace_go
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/nativelib/linux_arm64 -lfoldspace_go
#cgo darwin LDFLAGS: -lfoldspace_go
#include <stdlib.h>
#include "foldspace_go.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// BuiltWithFoldspace is whether this binary was compiled with the foldspace tag.
const BuiltWithFoldspace = true

// nativeCore implements Core directly against the C ABI declared in
// foldspace_go.h.
//
// The package's value types already mirror the ABI, constant for constant, so
// there is no intermediate Go API: each method is the C call plus the borrowing
// discipline cgo requires.
//
// Every pointer handed to the library is borrowed for the duration of the call,
// so Go memory is pinned for the call rather than copied into C. The one
// exception is a batch body, which the library owns and this package copies,
// because a Lease outlives the effect that carried it.
type nativeCore struct {
	client *C.foldspace_client
	// endpoints answers SenderCount and SenderClass, which the ABI does not
	// expose per sender.
	endpoints []Endpoint
}

// NewNativeCore constructs a Core backed by libfoldspace_go.
//
// Linux links the library vendored in nativelib. Elsewhere it comes from
// CGO_LDFLAGS, which `dda inv foldspace.build` reports.
//
// The ABI carries no negotiation, so a library built from a different revision
// than foldspace_go.h would read fields at the wrong offsets. The version check
// here is what turns that into a refusal rather than corruption.
func NewNativeCore(cfg Config) (Core, error) {
	if got, want := uint32(C.foldspace_abi_version()), uint32(C.FOLDSPACE_ABI_VERSION); got != want {
		return nil, fmt.Errorf("foldspace: the linked library implements ABI %d and this package was built against %d", got, want)
	}
	assertEncodingNumbering()
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("foldspace: at least one endpoint is required")
	}

	// Eviction of the dictionary and pattern tables is left disabled: Config
	// exposes no bounds for them, and a zeroed foldspace_eviction is ignored.
	native := C.foldspace_config{
		max_inflight_payloads:        C.uint64_t(cfg.MaxInflightPayloads),
		batch_capacity:               C.uint64_t(cfg.BatchCapacity),
		max_payload_bytes:            C.uint64_t(cfg.MaxPayloadBytes),
		content_encoding:             C.int(cfg.Compression),
		zstd_level:                   C.int(cfg.ZstdLevel),
		reconnect_backoff_base_nanos: C.uint64_t(cfg.ReconnectBackoffBase),
		reconnect_backoff_factor:     C.uint32_t(cfg.ReconnectBackoffFactor),
		reconnect_backoff_cap_nanos:  C.uint64_t(cfg.ReconnectBackoffCap),
		drain_timeout_nanos:          C.uint64_t(cfg.DrainTimeout),
		stream_lifetime_nanos:        C.uint64_t(cfg.StreamLifetime),
		first_payload_batch_id:       C.uint32_t(cfg.FirstPayloadBatchID),
		snapshot_batch_id:            C.uint32_t(cfg.SnapshotBatchID),
	}

	classes := make([]C.uint8_t, len(cfg.Endpoints))
	for i, e := range cfg.Endpoints {
		classes[i] = C.uint8_t(e.Class)
	}

	var client *C.foldspace_client
	code := C.foldspace_client_new(&native, &classes[0], C.size_t(len(classes)), &client)
	if code != C.FOLDSPACE_OK {
		return nil, fmt.Errorf("foldspace: %s", statusMessage(code))
	}

	core := &nativeCore{client: client, endpoints: append([]Endpoint(nil), cfg.Endpoints...)}
	// The library is freed by Close. A finalizer would not do: freeing it from
	// the collector's goroutine while a sender is inside a region is exactly
	// the concurrent entry the contract forbids.
	return core, nil
}

func (c *nativeCore) Start() Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_start(c.client, &p.native))
	return p.result()
}

func (c *nativeCore) HasCapacity() bool {
	var out C.uint8_t
	must(C.foldspace_has_capacity(c.client, &out))
	return out != 0
}

func (c *nativeCore) PushLog(record Record, nowNanos uint64, metadataID uint64, route Route) (Admission, Progress) {
	var pinner runtime.Pinner
	defer pinner.Unpin()

	native := C.foldspace_log_record{
		body:             pinBytes(&pinner, record.Body),
		timestamp_millis: C.int64_t(record.TimestampMillis),
		service:          pinString(&pinner, record.Service),
		status:           pinString(&pinner, record.Status),
		source:           pinString(&pinner, record.Source),
		hostname:         pinString(&pinner, record.Hostname),
		uuid:             pinString(&pinner, record.UUID),
	}
	native.tags, native.tags_len = pinStrings(&pinner, record.Tags)
	native.processing_tags, native.processing_tags_len = pinStrings(&pinner, record.ProcessingTags)

	p := c.progress(&pinner)
	var admission C.int
	must(C.foldspace_push_log(c.client, &native, C.uint64_t(nowNanos), C.uint64_t(metadataID), C.uint64_t(route), &admission, &p.native))
	return Admission(admission), p.result()
}

func (c *nativeCore) Flush() (Admission, Progress) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	var admission C.int
	must(C.foldspace_flush(c.client, &admission, &p.native))
	return Admission(admission), p.result()
}

func (c *nativeCore) BeginShutdown() (Admission, Progress) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	var admission C.int
	must(C.foldspace_begin_shutdown(c.client, &admission, &p.native))
	return Admission(admission), p.result()
}

func (c *nativeCore) Abandon() Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_force_shutdown(c.client, &p.native))
	return p.result()
}

func (c *nativeCore) IsDrained() bool {
	var out C.uint8_t
	must(C.foldspace_is_drained(c.client, &out))
	return out != 0
}

func (c *nativeCore) PollSender(sender SenderID) []Effect {
	var list *C.foldspace_effects
	must(C.foldspace_poll_sender(c.client, C.uint64_t(sender), &list))
	if list == nil {
		return nil
	}
	defer C.foldspace_effects_free(list)

	out := make([]Effect, 0, int(C.foldspace_effects_len(list)))
	for {
		native := C.foldspace_effects_next(list)
		if native == nil {
			return out
		}
		out = append(out, effectFrom(native))
		C.foldspace_effect_free(native)
	}
}

func (c *nativeCore) HandleStreamOpened(sender SenderID, stream StreamID) Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_handle_stream_opened(c.client, C.uint64_t(sender), C.uint64_t(stream), &p.native))
	return p.result()
}

func (c *nativeCore) HandleAck(sender SenderID, stream StreamID, batchID uint32, status int32) Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_handle_ack(c.client, C.uint64_t(sender), C.uint64_t(stream), C.uint32_t(batchID), C.int32_t(status), &p.native))
	return p.result()
}

func (c *nativeCore) HandleStreamError(sender SenderID, stream StreamID, message string) Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_handle_stream_error(c.client, C.uint64_t(sender), C.uint64_t(stream), pinString(&pinner, message), &p.native))
	return p.result()
}

func (c *nativeCore) HandleTimer(sender SenderID, stream StreamID, timer TimerKind) Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_handle_timer(c.client, C.uint64_t(sender), C.uint64_t(stream), C.int(timer), &p.native))
	return p.result()
}

func (c *nativeCore) PollNotifications() []Notification {
	var list *C.foldspace_notifications
	must(C.foldspace_poll_notifications(c.client, &list))
	if list == nil {
		return nil
	}
	defer C.foldspace_notifications_free(list)

	out := make([]Notification, 0, int(C.foldspace_notifications_len(list)))
	for {
		native := C.foldspace_notifications_next(list)
		if native == nil {
			return out
		}
		out = append(out, notificationFrom(native))
		C.foldspace_notification_free(native)
	}
}

func (c *nativeCore) TakeClockAdvance() time.Duration {
	var out C.uint64_t
	must(C.foldspace_take_clock_advance_nanos(c.client, &out))
	return time.Duration(out)
}

// Close frees the library. Calling it twice is harmless; calling anything else
// afterwards is not.
func (c *nativeCore) Close() {
	if c.client == nil {
		return
	}
	C.foldspace_client_free(c.client)
	c.client = nil
}

func (c *nativeCore) SenderCount() int { return len(c.endpoints) }

func (c *nativeCore) SenderClass(sender SenderID) SenderClass {
	return c.endpoints[sender].Class
}

// --- progress ---------------------------------------------------------------

// progressBuf owns the wake array the library writes into. The array is sized
// at the sender count, which the header states can never truncate, so a
// truncated result means the two disagree about how many senders exist.
type progressBuf struct {
	native C.foldspace_progress
	wake   []C.uint64_t
}

func (c *nativeCore) progress(pinner *runtime.Pinner) *progressBuf {
	p := &progressBuf{wake: make([]C.uint64_t, len(c.endpoints))}
	if len(p.wake) > 0 {
		pinner.Pin(&p.wake[0])
		p.native.wake = &p.wake[0]
		p.native.wake_capacity = C.size_t(len(p.wake))
	}
	pinner.Pin(p)
	return p
}

func (p *progressBuf) result() Progress {
	if p.native.wake_total > p.native.wake_len {
		panic(fmt.Sprintf("foldspace: %d senders woke but the wake array holds %d", p.native.wake_total, p.native.wake_capacity))
	}
	out := Progress{
		HasCapacity:        p.native.has_capacity != 0,
		NotificationsReady: p.native.notifications_ready != 0,
	}
	if n := int(p.native.wake_len); n > 0 {
		out.Wake = make([]SenderID, n)
		for i := 0; i < n; i++ {
			out.Wake[i] = SenderID(p.wake[i])
		}
	}
	return out
}

// --- effects and notifications ----------------------------------------------

func effectFrom(native *C.foldspace_effect) Effect {
	out := Effect{
		Kind:    EffectKind(C.foldspace_effect_kind(native)),
		Sender:  SenderID(C.foldspace_effect_sender(native)),
		Stream:  StreamID(C.foldspace_effect_stream(native)),
		After:   time.Duration(C.foldspace_effect_after_nanos(native)),
		BatchID: uint32(C.foldspace_effect_batch_id(native)),
	}
	if out.Kind == ScheduleTimer {
		out.Timer = TimerKind(C.foldspace_effect_timer_kind(native))
	}
	if lease := C.foldspace_effect_lease(native); lease != nil {
		// NewLease copies, so the library's bytes are needed only for that
		// copy and the lease is freed here rather than on release.
		out.Batch = NewLease(leaseBytes(lease), nil)
		C.foldspace_lease_free(lease)
	}
	if out.Kind == ReportError {
		var nativeErr C.foldspace_core_error
		must(C.foldspace_effect_error(native, &nativeErr))
		out.Err = &CoreError{
			Kind:            CoreErrorKind(nativeErr.kind),
			ExpectedBatchID: uint32(nativeErr.expected_batch_id),
			ActualBatchID:   uint32(nativeErr.actual_batch_id),
			BatchID:         uint32(nativeErr.batch_id),
			BatchStatus:     int32(nativeErr.batch_status),
			Message:         goString(nativeErr.message),
		}
	}
	return out
}

func leaseBytes(lease *C.foldspace_lease) []byte {
	n := int(C.foldspace_lease_len(lease))
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(C.foldspace_lease_bytes(lease))), n)
}

func notificationFrom(native *C.foldspace_notification) Notification {
	out := Notification{
		Kind:    NotificationKind(C.foldspace_notification_kind(native)),
		Records: uint64(C.foldspace_notification_records(native)),
		Bytes:   uint64(C.foldspace_notification_bytes(native)),
	}
	var sender C.uint64_t
	if C.foldspace_notification_sender(native, &sender) != 0 {
		out.Sender, out.HasSender = SenderID(sender), true
	}
	var count C.size_t
	if ids := C.foldspace_notification_metadata_ids(native, &count); ids != nil && count > 0 {
		out.MetadataIDs = append([]uint64(nil), unsafe.Slice((*uint64)(unsafe.Pointer(ids)), int(count))...)
	}
	return out
}

// --- borrowing helpers ------------------------------------------------------

// pinBytes lends b to the library for the duration of one call. A nil pointer
// means absent, which the library distinguishes from present and empty.
func pinBytes(pinner *runtime.Pinner, b []byte) C.foldspace_str {
	if len(b) == 0 {
		return C.foldspace_str{}
	}
	pinner.Pin(&b[0])
	return C.foldspace_str{ptr: (*C.uint8_t)(unsafe.Pointer(&b[0])), len: C.size_t(len(b))}
}

// pinString lends s to the library. An empty string is absent, matching
// Record's contract.
func pinString(pinner *runtime.Pinner, s string) C.foldspace_str {
	if s == "" {
		return C.foldspace_str{}
	}
	data := unsafe.StringData(s)
	pinner.Pin(data)
	return C.foldspace_str{ptr: (*C.uint8_t)(unsafe.Pointer(data)), len: C.size_t(len(s))}
}

func pinStrings(pinner *runtime.Pinner, values []string) (*C.foldspace_str, C.size_t) {
	if len(values) == 0 {
		return nil, 0
	}
	native := make([]C.foldspace_str, len(values))
	for i, v := range values {
		native[i] = pinString(pinner, v)
	}
	pinner.Pin(&native[0])
	return &native[0], C.size_t(len(native))
}

func goString(s C.foldspace_str) string {
	if s.ptr == nil || s.len == 0 {
		return ""
	}
	return C.GoStringN((*C.char)(unsafe.Pointer(s.ptr)), C.int(s.len))
}

// assertEncodingNumbering panics if types.go's Compression or NotificationKind
// constants have drifted from the linked header's own numbering. types.go
// cannot import "C" and stay buildable without the foldspace tag, so its
// constants are plain ints kept in sync by hand; this is what turns a drifted
// value into a refusal rather than corruption, the same role the ABI version
// check plays for the struct layout as a whole.
func assertEncodingNumbering() {
	if Compression(C.FOLDSPACE_ENCODING_DEFAULT) != Default ||
		Compression(C.FOLDSPACE_ENCODING_IDENTITY) != Identity ||
		Compression(C.FOLDSPACE_ENCODING_ZSTD) != Zstd {
		panic("foldspace: Compression constants in types.go do not match foldspace_go.h")
	}
	if NotificationKind(C.FOLDSPACE_NOTIFICATION_PAYLOAD_DURABLE) != PayloadDurable ||
		NotificationKind(C.FOLDSPACE_NOTIFICATION_PAYLOAD_ABANDONED) != PayloadAbandoned ||
		NotificationKind(C.FOLDSPACE_NOTIFICATION_DROPPED_STATS) != DroppedStats {
		panic("foldspace: NotificationKind constants in types.go do not match foldspace_go.h")
	}
}

// --- status -----------------------------------------------------------------

func statusMessage(code C.int) string {
	if message := C.foldspace_error_message(code); message != nil {
		return C.GoString(message)
	}
	return fmt.Sprintf("status %d", int(code))
}

// must panics on any status but OK. The whole invocation error surface is
// caller error — a null handle or an out-of-range sender — so there is nothing
// a caller could do with it at runtime that it should not have done at compile
// time. Construction is the exception and returns an error.
func must(code C.int) {
	if code != C.FOLDSPACE_OK {
		panic("foldspace: " + statusMessage(code))
	}
}
