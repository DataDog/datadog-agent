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
	"sync"
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

// sharedTokenizer is the pattern extractor's compiled automaton. It is large
// and holds no per-log state, so every client in the process is built from
// this one. It is never freed: foldspace_tokenizer_free must not run
// concurrently with a foldspace_client_new it is passed to, and a client can
// be built at any point in the process's life.
var sharedTokenizer = sync.OnceValues(func() (*C.foldspace_tokenizer, error) {
	var tokenizer *C.foldspace_tokenizer
	if code := C.foldspace_tokenizer_new(&tokenizer); code != C.FOLDSPACE_OK {
		return nil, fmt.Errorf("foldspace: %s", statusMessage(code))
	}
	return tokenizer, nil
})

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
	assertABINumbering()
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("foldspace: at least one endpoint is required")
	}

	// Seeded from the library's own defaults rather than built from a bare
	// struct literal, so dictionary_eviction and pattern_eviction come from
	// foldspace itself instead of being restated here and drifting from it.
	// Config exposes no fields for either; every assignment below overrides
	// a field Config does own, and leaves the rest, including both eviction
	// bounds, at what foldspace_default_config wrote.
	var native C.foldspace_config
	must(C.foldspace_default_config(&native))
	native.max_inflight_payloads = C.uint64_t(cfg.MaxInflightPayloads)
	native.batch_capacity = C.uint64_t(cfg.BatchCapacity)
	native.max_payload_bytes = C.uint64_t(cfg.MaxPayloadBytes)
	native.content_encoding = C.int(cfg.Compression)
	native.zstd_level = C.int(cfg.ZstdLevel)
	native.reconnect_backoff_base_nanos = C.uint64_t(cfg.ReconnectBackoffBase)
	native.reconnect_backoff_factor = C.uint32_t(cfg.ReconnectBackoffFactor)
	native.reconnect_backoff_cap_nanos = C.uint64_t(cfg.ReconnectBackoffCap)
	native.drain_timeout_nanos = C.uint64_t(cfg.DrainTimeout)
	native.stream_lifetime_nanos = C.uint64_t(cfg.StreamLifetime)
	native.pattern_sightings_before_admission = C.uint32_t(cfg.PatternSightingsBeforeAdmission)

	classes := make([]C.uint8_t, len(cfg.Endpoints))
	for i, e := range cfg.Endpoints {
		classes[i] = C.uint8_t(e.Class)
	}

	tokenizer, err := sharedTokenizer()
	if err != nil {
		return nil, err
	}

	var client *C.foldspace_client
	code := C.foldspace_client_new(&native, tokenizer, &classes[0], C.size_t(len(classes)), &client)
	if code != C.FOLDSPACE_OK {
		return nil, fmt.Errorf("foldspace: %s", statusMessage(code))
	}

	core := &nativeCore{client: client, endpoints: append([]Endpoint(nil), cfg.Endpoints...)}
	// The library is freed by Close. A finalizer would not do: freeing it from
	// the collector's goroutine while a sender is inside a region is exactly
	// the concurrent entry the contract forbids.
	return core, nil
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
	var list C.foldspace_effects
	must(C.foldspace_poll_sender(c.client, C.uint64_t(sender), &list))
	defer C.foldspace_effects_free(&list)
	if list.len == 0 {
		return nil
	}

	natives := unsafe.Slice(list.items, int(list.len))
	out := make([]Effect, len(natives))
	for i := range natives {
		out[i] = effectFrom(&natives[i])
	}
	return out
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

func (c *nativeCore) HandleStreamAbandoned(sender SenderID, stream StreamID, message string) Progress {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	p := c.progress(&pinner)
	must(C.foldspace_handle_stream_abandoned(c.client, C.uint64_t(sender), C.uint64_t(stream), pinString(&pinner, message), &p.native))
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
	var list C.foldspace_notifications
	must(C.foldspace_poll_notifications(c.client, &list))
	defer C.foldspace_notifications_free(&list)
	if list.len == 0 {
		return nil
	}

	natives := unsafe.Slice(list.items, int(list.len))
	out := make([]Notification, len(natives))
	for i := range natives {
		out[i] = notificationFrom(&natives[i])
	}
	return out
}

// SenderStats reads the library's counters without entering a sender's
// region: like foldspace_stats, foldspace_sender_stats takes no region lock.
func (c *nativeCore) SenderStats(sender SenderID) SenderCounts {
	var out C.foldspace_sender_counts
	must(C.foldspace_sender_stats(c.client, C.uint64_t(sender), &out))
	return SenderCounts{
		SentRecords:    uint64(out.sent_records),
		SentBytes:      uint64(out.sent_bytes),
		DroppedRecords: uint64(out.dropped_records),
		DroppedBytes:   uint64(out.dropped_bytes),
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

// effectFrom reads one effect in place from the list that carries it. Nothing
// it returns points into the list, which is freed once every effect is read.
func effectFrom(native *C.foldspace_effect) Effect {
	out := Effect{
		Kind:    EffectKind(native.kind),
		Sender:  SenderID(native.sender),
		Stream:  StreamID(native.stream),
		After:   time.Duration(native.after_nanos),
		BatchID: uint32(native.batch_id),
	}
	if out.Kind == ScheduleTimer {
		out.Timer = TimerKind(native.timer_kind)
	}
	if native.lease != nil {
		// NewLease copies, so the library's bytes are needed only for that
		// copy and the lease is freed here rather than on release.
		out.Batch = NewLease(strBytes(native.batch), nil)
		C.foldspace_lease_free(native.lease)
	}
	if out.Kind == ReportError {
		nativeErr := native.error
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

// strBytes views the library's bytes without copying. They are valid only
// until the list or lease that owns them is freed.
func strBytes(s C.foldspace_str) []byte {
	if s.ptr == nil || s.len == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(s.ptr)), int(s.len))
}

// notificationFrom copies the metadata ids out, because they point into the
// list that carries the notification.
func notificationFrom(native *C.foldspace_notification) Notification {
	out := Notification{Kind: NotificationKind(native.kind)}
	if native.metadata_ids != nil && native.metadata_ids_len > 0 {
		out.MetadataIDs = append([]uint64(nil), unsafe.Slice((*uint64)(unsafe.Pointer(native.metadata_ids)), int(native.metadata_ids_len))...)
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

// assertABINumbering panics if types.go's Compression, Admission, or
// NotificationKind constants have drifted from the linked header's own
// numbering. types.go cannot import "C" and stay buildable without the
// foldspace tag, so its constants are plain ints kept in sync by hand; this is
// what turns a drifted value into a refusal rather than corruption, the same
// role the ABI version check plays for the struct layout as a whole.
func assertABINumbering() {
	if Compression(C.FOLDSPACE_ENCODING_DEFAULT) != Default ||
		Compression(C.FOLDSPACE_ENCODING_IDENTITY) != Identity ||
		Compression(C.FOLDSPACE_ENCODING_ZSTD) != Zstd ||
		Compression(C.FOLDSPACE_ENCODING_JSPLIT) != JSplit {
		panic("foldspace: Compression constants in types.go do not match foldspace_go.h")
	}
	if Admission(C.FOLDSPACE_ADMISSION_ACCEPTED) != Accepted ||
		Admission(C.FOLDSPACE_ADMISSION_REFUSED) != Refused ||
		Admission(C.FOLDSPACE_ADMISSION_TOO_LARGE) != TooLarge ||
		Admission(C.FOLDSPACE_ADMISSION_SHUTTING_DOWN) != ShuttingDown ||
		Admission(C.FOLDSPACE_ADMISSION_EMPTY) != Empty ||
		Admission(C.FOLDSPACE_ADMISSION_UNROUTABLE) != Unroutable ||
		Admission(C.FOLDSPACE_ADMISSION_UNROUTED) != Unrouted {
		panic("foldspace: Admission constants in types.go do not match foldspace_go.h")
	}
	if NotificationKind(C.FOLDSPACE_NOTIFICATION_PAYLOAD_DURABLE) != PayloadDurable ||
		NotificationKind(C.FOLDSPACE_NOTIFICATION_PAYLOAD_ABANDONED) != PayloadAbandoned {
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
