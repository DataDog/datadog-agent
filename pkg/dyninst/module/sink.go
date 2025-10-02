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
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type sink struct {
	runtime      *runtimeImpl
	decoder      Decoder
	symbolicator symbol.Symbolicator
	programID    ir.ProgramID
	service      string
	logUploader  LogsUploader
	tree         *bufferTree
}

var _ dispatcher.Sink = &sink{}

// We don't want to be too noisy about decoding errors, but we do want to learn
// about them and we don't want to bail out completely.
var decodingErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

var noMatchingEventLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

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
			msg = dispatcher.Message{} // prevent release
			return nil
		}

		// If the buffer was full, mark the event to inform the user, and output
		// it directly.
		evHeader.Event_pairing_expectation =
			uint8(output.EventPairingExpectationBufferFull)
		fallthrough
	case output.EventPairingExpectationNone,
		output.EventPairingExpectationCallMapFull,
		output.EventPairingExpectationCallCountExceeded:
		entryEvent = msgEvent
	default:
		return fmt.Errorf("unknown event pairing expectation: %d", evHeader.Event_pairing_expectation)
	}
	decodedBytes, probe, err = s.decoder.Decode(decode.Event{
		EntryOrLine: entryEvent,
		Return:      returnEvent,
		ServiceName: s.service,
	}, s.symbolicator, decodedBytes)
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
