// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
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
	logUploader  LogsUploader
	tree         *bufferTree
	missingTypes missingTypeTracker

	// Probes is an ordered list of probes. The event header's probe_id is an
	// index into this list.
	probes []*ir.Probe
}

var _ dispatcher.Sink = &sink{}

// We don't want to be too noisy about decoding errors, but we do want to learn
// about them and we don't want to bail out completely.
var decodingErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

var noMatchingEventLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var eventPairingBufferFullLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var eventPairingCallMapFullLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var eventPairingCallCountExceededLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

func (s *sink) HandleEvent(msg dispatcher.Message) error {
	defer func() {
		if msg != (dispatcher.Message{}) {
			msg.Release()
		}
	}()
	var (
		decodedBytes []byte
		probe        ir.ProbeDefinition
		err          error
	)
	msgEvent := msg.Event()
	evHeader, err := msgEvent.Header()
	if err != nil {
		return fmt.Errorf("error getting event header: %w", err)
	}

	recordEventPairingIssue := func(
		stats *atomic.Uint64, limiter *rate.Limiter, issueMsg string,
	) {
		stats.Add(1)
		var probeID string
		if int(evHeader.Probe_id) < len(s.probes) {
			probeID = s.probes[evHeader.Probe_id].GetID()
		} else {
			probeID = fmt.Sprintf("unknown probeID %d", evHeader.Probe_id)
		}
		const format = "event pairing issue for probe %s: %s"
		if limiter.Allow() {
			log.Infof(format, probeID, issueMsg)
		} else {
			log.Tracef(format, probeID, issueMsg)
		}
	}
	var entryEvent, returnEvent output.Event
	switch output.EventPairingExpectation(evHeader.Event_pairing_expectation) {
	case output.EventPairingExpectationEntryPairingExpected:
		entryMsg, ok := s.tree.popMatchingEvent(eventKey{
			goid:           evHeader.Goid,
			stackByteDepth: evHeader.Stack_byte_depth,
			probeID:        evHeader.Probe_id,
		})
		// We expected to find a matching entry event but didn't. This could
		// happen if we ran out of buffer space for the entry event.
		if !ok {
			if noMatchingEventLogLimiter.Allow() {
				log.Warnf(
					"no matching event for goid %d, stackByteDepth %d, probeID %d",
					evHeader.Goid, evHeader.Stack_byte_depth, evHeader.Probe_id,
				)
			} else {
				log.Tracef(
					"no matching event for goid %d, stackByteDepth %d, probeID %d",
					evHeader.Goid, evHeader.Stack_byte_depth, evHeader.Probe_id,
				)
			}
			return nil
		}
		defer entryMsg.Release()
		entryEvent = entryMsg.Event()
		returnEvent = msgEvent
	case output.EventPairingExpectationReturnPairingExpected:
		if s.tree.addEvent(eventKey{
			goid:           evHeader.Goid,
			stackByteDepth: evHeader.Stack_byte_depth,
			probeID:        evHeader.Probe_id,
		}, msg) {
			// Record stack PCs for later use when the return event arrives.
			// This works around a bug where the return event may need the PCs
			// but doesn't have them.
			if stackPCs, err := msgEvent.StackPCs(); err == nil {
				s.decoder.ReportStackPCs(evHeader.Stack_hash, slices.Clone(stackPCs))
			}
			msg = dispatcher.Message{} // prevent release
			return nil
		}

		// If the buffer was full, mark the event to inform the user, and output
		// it directly.
		evHeader.Event_pairing_expectation =
			uint8(output.EventPairingExpectationBufferFull)
		recordEventPairingIssue(
			&s.runtime.stats.eventPairingBufferFull,
			eventPairingBufferFullLogLimiter,
			"userspace buffer capacity exceeded",
		)
		entryEvent = msgEvent
	case output.EventPairingExpectationCallMapFull:
		recordEventPairingIssue(
			&s.runtime.stats.eventPairingCallMapFull,
			eventPairingCallMapFullLogLimiter,
			"call map capacity exceeded",
		)
		entryEvent = msgEvent
	case output.EventPairingExpectationCallCountExceeded:
		recordEventPairingIssue(
			&s.runtime.stats.eventPairingCallCountExceeded,
			eventPairingCallCountExceededLogLimiter,
			"maximum call count exceeded",
		)
		entryEvent = msgEvent
	case output.EventPairingExpectationNone,
		output.EventPairingExpectationNoneInlined,
		output.EventPairingExpectationNoneNoBody:
		entryEvent = msgEvent
	default:
		return fmt.Errorf("unknown event pairing expectation: %d", evHeader.Event_pairing_expectation)
	}
	decodedBytes, probe, err = s.decoder.Decode(decode.Event{
		EntryOrLine: entryEvent,
		Return:      returnEvent,
		ServiceName: s.service,
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
			return nil
		}
		if decodingErrorLogLimiter.Allow() {
			log.Warnf(
				"failed to decode event in service %s: %v",
				s.service, err,
			)
		} else {
			log.Tracef(
				"failed to decode event in service %s: %v",
				s.service, err,
			)
		}
		// TODO: Report failures to the controller to remove the relevant probe
		// or program.
		return nil
	}
	s.runtime.setProbeMaybeEmitting(s.programID, probe)
	s.logUploader.Enqueue(json.RawMessage(decodedBytes))
	if missingTypes := s.missingTypes.drain(); len(missingTypes) > 0 {
		s.runtime.actuator.ReportMissingTypes(s.processID, missingTypes)
	}
	return nil
}

func (s *sink) Close() {
	if s.logUploader != nil {
		s.logUploader.Close()
	}
	if closer, ok := s.symbolicator.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Warnf("failed to close symbolicator: %v", err)
		}
	}
	s.tree.close()
}
