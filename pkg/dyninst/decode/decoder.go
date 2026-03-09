// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// We don't want to be too noisy about symbolication errors, but we do want to learn
// about them and we don't want to bail out completely.
var symbolicateErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

// TypeNameResolver resolves type names from type IDs as communicated by the
// probe regarding types in interfaces.
type TypeNameResolver interface {
	ResolveTypeName(typeID gotype.TypeID) (string, error)
}

// MissingTypeCollector receives notifications about types that were
// encountered at runtime in interfaces but were not included in the IR
// program's type registry. Implementations must be safe for use from a
// single goroutine (the decoder is not thread-safe).
type MissingTypeCollector interface {
	RecordMissingType(typeName string)
}

// GoTypeNameResolver is a TypeNameResolver that uses a gotype.Table to resolve
// type names.
type GoTypeNameResolver gotype.Table

// ResolveTypeName resolves the name of a type from a type ID.
func (r *GoTypeNameResolver) ResolveTypeName(typeID gotype.TypeID) (string, error) {
	t, err := (*gotype.Table)(r).ParseGoType(typeID)
	if err != nil {
		return "", err
	}
	// TODO: Note that this type name is not going to be fully qualified! In
	// order to make it match the type names we get from dwarf, we'll need to do
	// more work. This conversion is not trivial. It's likely better to instead
	// build a lookup table from the go runtime types to the type names we get
	// from dwarf -- but doing so will require using disk space or something
	// like that.
	//
	// As we build better name parsing support, it becomes more plausible to
	// do the conversion.
	return t.Name().Name(), nil
}

// noopMissingTypeCollector is a no-op implementation of MissingTypeCollector
// used when no collector is provided.
type noopMissingTypeCollector struct{}

func (noopMissingTypeCollector) RecordMissingType(string) {}

// Decoder decodes the output of the BPF program into a JSON format.
// It is not guaranteed to be thread-safe.
type Decoder struct {
	// These fields are initialized on decoder creation and are shared between messages.
	program              *ir.Program
	decoderTypes         map[ir.TypeID]decoderType
	probeEvents          map[ir.TypeID]probeEvent
	stackPCs             map[uint64][]uint64
	typesByGoRuntimeType map[uint32]ir.TypeID
	typeNameResolver     TypeNameResolver
	approximateBootTime  time.Time

	// These fields are initialized and reset for each message.
	message     message
	entryOrLine captureEvent
	_return     captureEvent
	line        lineCaptureData
	messageData messageData
}

// ReportStackPCs reports the program counters of the stack trace for a
// given stack hash.
func (d *Decoder) ReportStackPCs(stackHash uint64, stackPCs []uint64) {
	if len(stackPCs) > 0 {
		d.stackPCs[stackHash] = stackPCs
	}
}

// NewDecoder creates a new Decoder for the given program.
func NewDecoder(
	program *ir.Program,
	typeNameResolver TypeNameResolver,
	approximateBootTime time.Time,
) (*Decoder, error) {
	decoder := &Decoder{
		program:              program,
		decoderTypes:         make(map[ir.TypeID]decoderType, len(program.Types)),
		probeEvents:          make(map[ir.TypeID]probeEvent),
		stackPCs:             make(map[uint64][]uint64),
		typesByGoRuntimeType: make(map[uint32]ir.TypeID),
		typeNameResolver:     typeNameResolver,
		approximateBootTime:  approximateBootTime,

		message: message{},
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			decoder.probeEvents[event.Type.ID] = probeEvent{
				event: event,
				probe: probe,
			}
		}
	}
	for _, t := range program.Types {
		decoderType, err := newDecoderType(t, program.Types)
		if err != nil {
			return nil, fmt.Errorf("error getting decoder type for type %s: %w", t.GetName(), err)
		}
		id := t.GetID()
		decoder.decoderTypes[id] = decoderType
		if goRuntimeType, ok := t.GetGoRuntimeType(); ok {
			decoder.typesByGoRuntimeType[goRuntimeType] = id
		}
	}
	decoder.entryOrLine.encodingContext = encodingContext{
		typesByID:            decoder.decoderTypes,
		typesByGoRuntimeType: decoder.typesByGoRuntimeType,
		typeResolver:         typeNameResolver,
		dataItems:            make(map[typeAndAddr]output.DataItem),
		currentlyEncoding:    make(map[typeAndAddr]struct{}),
	}
	decoder._return.encodingContext = encodingContext{
		typesByID:            decoder.decoderTypes,
		typesByGoRuntimeType: decoder.typesByGoRuntimeType,
		typeResolver:         typeNameResolver,
		dataItems:            make(map[typeAndAddr]output.DataItem),
		currentlyEncoding:    make(map[typeAndAddr]struct{}),
	}
	return decoder, nil
}

// typeAndAddr is a type and address pair. It is used to uniquely identify a data item in the data items map.
// Addresses may not be unique to a type, for example if an address is taken for the first field of a struct.
type typeAndAddr struct {
	irType uint32
	addr   uint64
}

// Decode decodes the output Event from the BPF program into a JSON format
// the `output` parameter is appended to and returned as the final output.
// It is not thread-safe. If missingTypes is nil, missing types are silently
// ignored.
func (d *Decoder) Decode(
	event Event,
	symbolicator symbol.Symbolicator,
	missingTypes MissingTypeCollector,
	buf []byte,
) (_ []byte, probe ir.ProbeDefinition, err error) {
	defer d.resetForNextMessage()
	if missingTypes == nil {
		missingTypes = noopMissingTypeCollector{}
	}
	d.entryOrLine.missingTypeCollector = missingTypes
	d._return.missingTypeCollector = missingTypes
	defer func() {
		r := recover()
		switch r := r.(type) {
		case nil:
		case error:
			err = fmt.Errorf("Decode: panic: %w", r)
		default:
			err = fmt.Errorf("Decode: panic: %v\n%s", r, debug.Stack())
		}
	}()
	probe, err = d.message.init(d, event, symbolicator)
	if err != nil {
		return buf, nil, err
	}
	b := bytes.NewBuffer(buf)
	enc := jsontext.NewEncoder(b)
	var numExpressions int
	if captures := d.message.Debugger.Snapshot.Captures; captures != nil {
		if captures.Entry != nil {
			numExpressions = len(captures.Entry.rootType.Expressions)
			if captures.Return != nil {
				numExpressions += len(captures.Return.rootType.Expressions)
			}
		} else if captures.Lines != nil {
			numExpressions = len(captures.Lines.capture.rootType.Expressions)
		}
	}
	// We loop here because when evaluation errors occur, we reduce the amount of data we attempt
	// to encode and then try again after resetting the buffer.
	for range numExpressions + 1 { // +1 for the initial attempt
		err = json.MarshalEncode(enc, &d.message)
		if errors.Is(err, errEvaluation) {
			b = bytes.NewBuffer(buf)
			enc.Reset(b)
			continue
		} else if err != nil {
			return buf, probe, fmt.Errorf("error marshaling snapshot message: %w", err)
		}
		break
	}
	return b.Bytes(), probe, err
}

func (d *Decoder) resetForNextMessage() {
	clear(d.entryOrLine.dataItems)
	d.entryOrLine.clear()
	d.entryOrLine.missingTypeCollector = nil
	d.line.clear()
	d._return.clear()
	d._return.missingTypeCollector = nil
	d.messageData = messageData{}
	d.message = message{}
}

// Event wraps the output Event from the BPF program. It also adds fields
// that are not present in the BPF program.
type Event struct {
	Probe       *ir.Probe
	EntryOrLine output.Event
	Return      output.Event
	ServiceName string
}

type message struct {
	Service   string           `json:"service"`
	DDSource  ddDebuggerSource `json:"ddsource"`
	Logger    logger           `json:"logger"`
	Debugger  debuggerData     `json:"debugger"`
	Timestamp int              `json:"timestamp"`
	Duration  uint64           `json:"duration,omitzero"`
	Message   *messageData     `json:"message,omitempty"`
}

// populateStackPCsIfMissing populates the decoder's stackPCs map with stack PCs
// from the given event if the stack hash is not already present.
func populateStackPCsIfMissing(
	probe *ir.Probe,
	decoder *Decoder,
	stackHash uint64,
	event output.Event,
	eventType string,
) {
	if _, ok := decoder.stackPCs[stackHash]; ok {
		return
	}
	stackPCs, err := event.StackPCs()
	if err != nil {
		if symbolicateErrorLogLimiter.Allow() {
			log.Errorf(
				"error getting stack pcs from %s event for probe %s: %v",
				eventType, probe.GetID(), err,
			)
		} else {
			log.Tracef(
				"error getting stack pcs from %s event for probe %s: %v",
				eventType, probe.GetID(), err,
			)
		}
		return
	}
	if len(stackPCs) > 0 {
		decoder.stackPCs[stackHash] = slices.Clone(stackPCs)
	}
}

func (s *message) init(
	decoder *Decoder,
	event Event,
	symbolicator symbol.Symbolicator,
) (ir.ProbeDefinition, error) {
	s.Service = event.ServiceName
	s.Debugger = debuggerData{
		Snapshot: snapshotData{
			ID:       uuid.New(),
			Language: "go",
		},
		EvaluationErrors: []evaluationError{},
	}
	if event.EntryOrLine == nil {
		return nil, errors.New("entry event is nil")
	}
	if err := decoder.entryOrLine.init(
		event.EntryOrLine, decoder.program.Types, &s.Debugger.EvaluationErrors,
	); err != nil {
		return nil, err
	}
	probeEvent := decoder.probeEvents[decoder.entryOrLine.rootType.ID]
	probe := probeEvent.probe
	header, err := event.EntryOrLine.Header()
	if err != nil {
		return probe, fmt.Errorf("error getting header %w", err)
	}
	switch probeEvent.event.Kind {
	case ir.EventKindEntry:
		s.Debugger.Snapshot.captures.Entry = &decoder.entryOrLine
	case ir.EventKindLine:
		decoder.line.sourceLine = probeEvent.event.SourceLine
		decoder.line.capture = &decoder.entryOrLine
		s.Debugger.Snapshot.captures.Lines = &decoder.line
	}
	var returnHeader *output.EventHeader
	var durationMissingReason *string
	if event.Return != nil {
		if err := decoder._return.init(
			event.Return, decoder.program.Types, &s.Debugger.EvaluationErrors,
		); err != nil {
			return nil, fmt.Errorf("error initializing return event: %w", err)
		}
		returnProbeEvent := decoder.probeEvents[decoder._return.rootType.ID]
		if returnProbeEvent.probe != probe {
			return nil, errors.New("return probe event has different probe than entry probe")
		}
		returnHeader, err = event.Return.Header()
		if err != nil {
			return nil, fmt.Errorf("error getting return header %w", err)
		}
		s.Duration = uint64(returnHeader.Ktime_ns - header.Ktime_ns)
		s.Debugger.Snapshot.captures.Return = &decoder._return
	} else {
		// Check if we expected a return event but didn't get one.
		pairingExpectation := output.EventPairingExpectation(
			header.Event_pairing_expectation,
		)
		var reason string
		switch pairingExpectation {
		case output.EventPairingExpectationReturnPairingExpected:
			reason = "return event not received"
		case output.EventPairingExpectationBufferFull:
			reason = "userspace buffer capacity exceeded"
		case output.EventPairingExpectationCallMapFull:
			reason = "call map capacity exceeded"
		case output.EventPairingExpectationCallCountExceeded:
			reason = "maximum call count exceeded"
		case output.EventPairingExpectationNoneInlined:
			reason = "function was inlined"
		case output.EventPairingExpectationNoneNoBody:
			reason = "function has no body"
		}
		log.Tracef("no return reason: %v pairing expectation: %v", reason, pairingExpectation)
		// The choice to use @duration here is somewhat arbitrary; we want to
		// choose something that definitely can't collide with a real variable
		// and is evocative of the thing that is missing. Indeed we know in this
		// situation we will never @duration, so it seems like a good choice.
		const missingReturnReasonExpression = "@duration"
		if reason != "" {
			message := "not available: " + reason
			s.Debugger.EvaluationErrors = append(
				s.Debugger.EvaluationErrors,
				evaluationError{
					Expression: missingReturnReasonExpression,
					Message:    message,
				},
			)
			durationMissingReason = &message
		}
	}

	s.Debugger.Snapshot.Timestamp = int(decoder.approximateBootTime.Add(
		time.Duration(header.Ktime_ns),
	).UnixMilli())
	s.Timestamp = s.Debugger.Snapshot.Timestamp

	// Unconditionally populate stackPCs map for any event with stack PCs.
	populateStackPCsIfMissing(
		probe, decoder, header.Stack_hash, event.EntryOrLine, "entry",
	)
	if returnHeader != nil {
		populateStackPCsIfMissing(
			probe, decoder, returnHeader.Stack_hash, event.Return, "return",
		)
	}

	if probe.GetKind() == ir.ProbeKindSnapshot || probe.GetKind() == ir.ProbeKindCaptureExpression {
		stackHeader := header
		if returnHeader != nil {
			stackHeader = returnHeader
		}
		stackPCs, ok := decoder.stackPCs[stackHeader.Stack_hash]
		if !ok {
			s.Debugger.EvaluationErrors = append(s.Debugger.EvaluationErrors,
				evaluationError{
					Expression: "Stacktrace",
					Message:    "no stack pcs found",
				})
		}
		var stackFrames []symbol.StackFrame
		if len(stackPCs) > 0 {
			stackFrames, err = symbolicator.Symbolicate(stackPCs, stackHeader.Stack_hash)
			if err != nil {
				if symbolicateErrorLogLimiter.Allow() {
					log.Errorf("error symbolicating stack for probe %s: %v", probe.GetID(), err)
				} else {
					log.Tracef("error symbolicating stack for probe %s: %v", probe.GetID(), err)
				}
				s.Debugger.EvaluationErrors = append(s.Debugger.EvaluationErrors,
					evaluationError{
						Expression: "Stacktrace",
						Message:    err.Error(),
					},
				)
			}
		}

		s.Debugger.Snapshot.stack.frames = stackFrames
		s.Debugger.Snapshot.Stack = &s.Debugger.Snapshot.stack
		s.Debugger.Snapshot.Captures = &s.Debugger.Snapshot.captures
	}

	switch where := probe.GetWhere().(type) {
	case ir.FunctionWhere:
		s.Debugger.Snapshot.Probe.Location.Method = where.Location()
		s.Logger.Method = where.Location()
	case ir.LineWhere:
		function, file, lineStr := where.Line()
		line, err := strconv.Atoi(lineStr)
		if err != nil {
			return probe, fmt.Errorf("invalid line number: %v", lineStr)
		}
		s.Debugger.Snapshot.Probe.Location.Method = function
		s.Debugger.Snapshot.Probe.Location.File = file
		s.Debugger.Snapshot.Probe.Location.Line = line
		s.Logger.Method = function
	default:
		return probe, fmt.Errorf(
			"probe %s is not on a supported location: %T",
			probe.GetID(), where,
		)
	}

	s.Logger.Version = probe.GetVersion()
	s.Logger.ThreadID = int(header.Goid)
	s.Debugger.Snapshot.Probe.ID = probe.GetID()

	if probe.Template != nil {
		decoder.messageData = messageData{
			entryOrLine: &decoder.entryOrLine,
			_return:     &decoder._return,
			template:    probe.Template,
		}
		s.Message = &decoder.messageData
		if s.Duration != 0 {
			s.Message.duration = &s.Duration
		} else if durationMissingReason != nil {
			s.Message.durationMissingReason = durationMissingReason
		}
	}

	return probe, nil
}
