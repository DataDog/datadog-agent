// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/eventbuf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/jsonprune"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// missingTypeTracker collects type names that the decoder encounters in
// interface values but cannot find in the IR type registry. It implements
// decode.MissingTypeCollector and is drained by the sink after each Decode.
type missingTypeTracker struct {
	typeSet map[string]struct{}
	nameBuf []string
}

// RecordMissingType implements decode.MissingTypeCollector.
func (t *missingTypeTracker) RecordMissingType(typeName string) {
	if t.typeSet == nil {
		t.typeSet = make(map[string]struct{})
	}
	t.typeSet[typeName] = struct{}{}
}

// drain returns the accumulated type names and resets the tracker.
// Returns nil if no types were collected. The returned slice is valid
// only until the next call to drain.
func (t *missingTypeTracker) drain() []string {
	if len(t.typeSet) == 0 {
		return nil
	}
	for name := range t.typeSet {
		t.nameBuf = append(t.nameBuf, name)
	}
	clear(t.typeSet)
	sort.Strings(t.nameBuf)
	ret := t.nameBuf
	t.nameBuf = t.nameBuf[:0]
	return ret
}

type sink struct {
	runtime      *runtimeImpl
	decoder      Decoder
	symbolicator symbol.Symbolicator
	programID    ir.ProgramID
	processID    actuator.ProcessID
	service      string
	processTags  string
	logUploader  LogsUploader

	// mu guards buffer, missingTypes, and appliedCutoffNs. HandleEvent and
	// HandleDropNotification arrive from the dispatcher's reader goroutines
	// (one per ringbuf), and EvictOlderThan arrives from the actuator
	// goroutine; mu serializes them all.
	mu              sync.Mutex
	buffer          *eventbuf.Buffer
	missingTypes    missingTypeTracker
	appliedCutoffNs uint64

	// Probes is an ordered list of probes. The event header's probe_id is an
	// index into this list.
	probes []*ir.Probe
}

var _ dispatcher.Sink = &sink{}

// We don't want to be too noisy about decoding errors, but we do want to learn
// about them and we don't want to bail out completely.
var decodingErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

var eventPairingCallMapFullLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var eventPairingCallCountExceededLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var eventPairingConditionFailedLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// keyFromHeader builds an eventbuf.Key from a (validated) event header.
func keyFromHeader(h *output.EventHeader) eventbuf.Key {
	return eventbuf.Key{
		Goid:           h.Goid,
		StackByteDepth: h.Stack_byte_depth,
		ProbeID:        h.Probe_id,
		EntryKtime:     h.Entry_ktime_ns,
	}
}

// HandleEvent routes a single message from the primary ringbuf through the
// event buffer, emitting decoded output whenever an invocation becomes
// complete.
func (s *sink) HandleEvent(msg dispatcher.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev := msg.Event()
	h, err := ev.Header()
	if err != nil {
		msg.Release()
		return fmt.Errorf("error getting event header: %w", err)
	}

	key := keyFromHeader(h)
	expectation := output.EventPairingExpectation(h.Event_pairing_expectation)

	switch expectation {
	case output.EventPairingExpectationConditionFailed:
		// BPF-sent signal: the return condition evaluated to false, so
		// discard any buffered entry for this invocation without emitting.
		s.buffer.Discard(key)
		s.recordEventPairingIssue(
			&s.runtime.stats.eventPairingConditionFailed,
			eventPairingConditionFailedLogLimiter,
			"return condition failed",
			h.Probe_id,
		)
		msg.Release()
		s.postMutate()
		return nil
	case output.EventPairingExpectationCallMapFull:
		// BPF ran out of room in the in_progress_calls map. The entry event
		// is emitted standalone (no return will come) with an operator log.
		s.recordEventPairingIssue(
			&s.runtime.stats.eventPairingCallMapFull,
			eventPairingCallMapFullLogLimiter,
			"call map capacity exceeded",
			h.Probe_id,
		)
	case output.EventPairingExpectationCallCountExceeded:
		s.recordEventPairingIssue(
			&s.runtime.stats.eventPairingCallCountExceeded,
			eventPairingCallCountExceededLogLimiter,
			"maximum call count exceeded",
			h.Probe_id,
		)
	}

	// Everything else carries fragment data. Route it through the buffer.
	side, expectReturn := sideFromExpectation(expectation)
	isFinal := !h.HasMoreFragments()
	// Record stack PCs on the first fragment of an entry that expects a
	// return; the return-side decode will use them.
	if side == eventbuf.Entry && expectReturn && h.Continuation_seq == 0 {
		if stackPCs, err := ev.StackPCs(); err == nil {
			s.decoder.ReportStackPCs(h.Stack_hash, slices.Clone(stackPCs))
		}
	}
	ready, done := s.buffer.AddFragment(
		key, wrapMessage(msg), side, h.Continuation_seq, isFinal, expectReturn,
	)
	if done {
		s.emit(ready)
	}
	s.postMutate()
	return nil
}

// HandleDropNotification applies a side-channel drop notification to the
// event buffer, finalizing the invocation it references if the resulting
// state is now complete. Blocks on s.mu so a quiescent probe still gets its
// truncated capture emitted promptly without waiting for the next main-channel
// event.
func (s *sink) HandleDropNotification(n output.DropNotification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := eventbuf.Key{
		Goid:           n.Goid,
		StackByteDepth: n.Stack_byte_depth,
		ProbeID:        n.Probe_id,
		EntryKtime:     n.Entry_ktime_ns,
	}
	var ready eventbuf.Ready
	var done bool
	switch output.DropReason(n.Drop_reason) {
	case output.DropReasonReturnLost:
		ready, done = s.buffer.NoteReturnLost(key)
	case output.DropReasonPartialEntry:
		ready, done = s.buffer.NotePartial(key, eventbuf.Entry, n.Last_seq)
	case output.DropReasonPartialReturn:
		ready, done = s.buffer.NotePartial(key, eventbuf.Return, n.Last_seq)
	default:
		log.Errorf("unknown drop reason %d", n.Drop_reason)
		return
	}
	if done {
		s.emit(ready)
	}
}

// postMutate runs after each buffer mutation. It drains any Readys the
// buffer surfaced as part of budget-driven eviction (triggered when an
// AddFragment exceeds the shared byte ceiling and forced the buffer to
// evict its oldest entries). Time-based eviction runs separately via
// EvictOlderThan, called by the actuator.
func (s *sink) postMutate() {
	for _, r := range s.buffer.TakePendingBudgetEvictions() {
		s.emit(r)
	}
}

// EvictOlderThan finalizes any buffered entries whose invocation predates
// cutoffKtimeNs. Called from the actuator goroutine when BPF reported that
// at least one drop notification was itself lost and the grace window has
// elapsed. The cutoff is monotonic: repeated calls with a non-increasing
// cutoff are a no-op.
func (s *sink) EvictOlderThan(cutoffKtimeNs uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cutoffKtimeNs <= s.appliedCutoffNs {
		return
	}
	s.appliedCutoffNs = cutoffKtimeNs
	for _, r := range s.buffer.EvictOlderThan(cutoffKtimeNs) {
		s.emit(r)
	}
}

// emit decodes and uploads the data in ready. Releases ready's messages
// after decode.
func (s *sink) emit(ready eventbuf.Ready) {
	defer func() {
		if ready.Entry != nil {
			ready.Entry.Release()
		}
		if ready.Return != nil {
			ready.Return.Release()
		}
	}()

	entry, ret := fragmentedEvents(ready)
	if entry == nil {
		// Nothing to decode (e.g. a zombie entry with no fragments). Skip.
		return
	}

	// decodedBytes is a fresh slice for each call — the log uploader
	// takes ownership of the returned bytes, so reusing a per-sink buffer
	// would corrupt previously-enqueued events on the next overwrite.
	var decodedBytes []byte
	decoded, probe, err := s.decoder.Decode(decode.Event{
		EntryOrLine: entry,
		Return:      ret,
		ServiceName: s.service,
		ProcessTags: s.processTags,
		Truncated:   ready.EntryTruncated || ready.ReturnTruncated,
	}, s.symbolicator, &s.missingTypes, decodedBytes)
	if err != nil {
		if probe != nil {
			if reported := s.runtime.reportProbeError(
				s.programID, probe, err, "DecodeFailed",
			); reported {
				log.Warnf(
					"failed to report probe error for probe %s in service %s: %v",
					probe.GetID(), s.service, err,
				)
			}
			return
		}
		if decodingErrorLogLimiter.Allow() {
			log.Warnf("failed to decode event in service %s: %v", s.service, err)
		} else {
			log.Tracef("failed to decode event in service %s: %v", s.service, err)
		}
		return
	}
	s.runtime.setProbeMaybeEmitting(s.programID, probe)
	if missingTypes := s.missingTypes.drain(); len(missingTypes) > 0 {
		s.runtime.actuator.ReportMissingTypes(s.processID, missingTypes)
	}
	decoded = jsonprune.Prune(decoded, jsonprune.MaxSnapshotBytes)
	s.logUploader.Enqueue(decoded)
}

func (s *sink) recordEventPairingIssue(
	stats *atomic.Uint64, limiter *rate.Limiter, issueMsg string, probeIdx uint32,
) {
	stats.Add(1)
	var probeID string
	if int(probeIdx) < len(s.probes) {
		probeID = s.probes[probeIdx].GetID()
	} else {
		probeID = fmt.Sprintf("unknown probeID %d", probeIdx)
	}
	const format = "event pairing issue for probe %s: %s"
	if limiter.Allow() {
		log.Infof(format, probeID, issueMsg)
	} else {
		log.Tracef(format, probeID, issueMsg)
	}
}

// fragmentedEvents returns the entry and return FragmentedEvent views to
// hand to the decoder. Return may be nil when ready has no return side.
func fragmentedEvents(ready eventbuf.Ready) (entry, ret output.FragmentedEvent) {
	if ready.Entry != nil {
		entry = ready.Entry
	}
	if ready.Return != nil {
		ret = ready.Return
	}
	return entry, ret
}

// sideFromExpectation returns the buffer side the event feeds into, plus
// whether the invocation expects a return (entry-side only).
//
// For unknown expectations, defaults to (Entry, false) — the decoder will
// see the event as a standalone. This keeps the sink resilient to future
// BPF additions of expectations.
func sideFromExpectation(e output.EventPairingExpectation) (eventbuf.Side, bool) {
	switch e {
	case output.EventPairingExpectationReturnPairingExpected:
		return eventbuf.Entry, true
	case output.EventPairingExpectationEntryPairingExpected:
		return eventbuf.Return, false
	case output.EventPairingExpectationNone,
		output.EventPairingExpectationNoneInlined,
		output.EventPairingExpectationNoneNoBody,
		output.EventPairingExpectationCallMapFull,
		output.EventPairingExpectationCallCountExceeded:
		return eventbuf.Entry, false
	}
	return eventbuf.Entry, false
}

func (s *sink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drain the buffer through emit before closing the uploader, otherwise
	// emit's Enqueue calls land on a stopped batcher and are silently dropped.
	for _, r := range s.buffer.Close() {
		s.emit(r)
	}
	if s.logUploader != nil {
		s.logUploader.Close()
	}
	if closer, ok := s.symbolicator.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Warnf("failed to close symbolicator: %v", err)
		}
	}
}
