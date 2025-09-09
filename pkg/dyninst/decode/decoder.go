// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/uuid"
	pkgerrors "github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

// TypeNameResolver resolves type names from type IDs as communicated by the
// probe regarding types in interfaces.
type TypeNameResolver interface {
	ResolveTypeName(typeID gotype.TypeID) (string, error)
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

// Decoder decodes the output of the BPF program into a JSON format.
// It is not guaranteed to be thread-safe.
type Decoder struct {
	// These fields are initialized on decoder creation and are shared between messages.
	program              *ir.Program
	decoderTypes         map[ir.TypeID]decoderType
	probeEvents          map[ir.TypeID]probeEvent
	stackFrames          map[uint64][]symbol.StackFrame
	typesByGoRuntimeType map[uint32]ir.TypeID
	typeNameResolver     TypeNameResolver

	// These fields are initialized and reset for each message.
	snapshotMessage   snapshotMessage
	dataItems         map[typeAndAddr]output.DataItem
	currentlyEncoding map[typeAndAddr]struct{}
}

// NewDecoder creates a new Decoder for the given program.
func NewDecoder(
	program *ir.Program,
	typeNameResolver TypeNameResolver,
) (*Decoder, error) {
	decoder := &Decoder{
		program:              program,
		decoderTypes:         make(map[ir.TypeID]decoderType, len(program.Types)),
		probeEvents:          make(map[ir.TypeID]probeEvent),
		stackFrames:          make(map[uint64][]symbol.StackFrame),
		typesByGoRuntimeType: make(map[uint32]ir.TypeID),
		typeNameResolver:     typeNameResolver,

		snapshotMessage:   snapshotMessage{},
		dataItems:         make(map[typeAndAddr]output.DataItem),
		currentlyEncoding: make(map[typeAndAddr]struct{}),
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
	return decoder, nil
}

// typeAndAddr is a type and address pair. It is used to uniquely identify a data item in the data items map.
// Addresses may not be unique to a type, for example if an address is taken for the first field of a struct.
type typeAndAddr struct {
	irType uint32
	addr   uint64
}

// Decode decodes the output Event from the BPF program into a JSON format to the specified output writer.
func (d *Decoder) Decode(
	event Event,
	symbolicator symbol.Symbolicator,
	out io.Writer,
) (probe ir.ProbeDefinition, err error) {
	defer d.resetForNextMessage()
	defer func() {
		r := recover()
		switch r := r.(type) {
		case nil:
		case error:
			err = pkgerrors.Wrap(r, "Decode: panic")
		default:
			err = pkgerrors.Errorf("Decode: panic: %v\n%s", r, debug.Stack())
		}
	}()
	probe, err = d.snapshotMessage.init(d, event, symbolicator)
	if err != nil {
		return nil, err
	}
	err = json.MarshalWrite(out, &d.snapshotMessage)
	if err != nil {
		err = fmt.Errorf("error marshaling snapshot message: %w", err)
	}
	return probe, err
}

func (d *Decoder) resetForNextMessage() {
	clear(d.dataItems)
	d.snapshotMessage = snapshotMessage{}
}

// Event wraps the output Event from the BPF program. It also adds fields
// that are not present in the BPF program.
type Event struct {
	output.Event
	ServiceName string
}

type snapshotMessage struct {
	Service   string           `json:"service"`
	DDSource  ddDebuggerSource `json:"ddsource"`
	Logger    logger           `json:"logger"`
	Debugger  debuggerData     `json:"debugger"`
	Timestamp int              `json:"timestamp"`

	rootData []byte
}

func (s *snapshotMessage) init(
	decoder *Decoder,
	event Event,
	symbolicator symbol.Symbolicator,
) (ir.ProbeDefinition, error) {
	s.Service = event.ServiceName
	s.Debugger.Snapshot = snapshotData{
		decoder:  decoder,
		ID:       uuid.New(),
		Language: "go",
	}

	var rootType *ir.EventRootType
	var probe ir.ProbeDefinition

	for item, err := range event.Event.DataItems() {
		if err != nil {
			return probe, fmt.Errorf("error getting data items: %w", err)
		}
		if rootType == nil {
			s.rootData = item.Data()
			rootTypeID := ir.TypeID(item.Header().Type)
			var ok bool
			rootType, ok = decoder.program.Types[rootTypeID].(*ir.EventRootType)
			if !ok {
				return nil, errors.New("expected event of type root first")
			}
			irProbe, ok := decoder.probeEvents[rootTypeID]
			if !ok {
				return probe, fmt.Errorf("no probe found for root type %v", rootTypeID)
			}
			probe = irProbe.probe
			continue
		}
		// We need to keep track of the address reference count for each data item.
		// This is used to avoid infinite recursion when encoding pointers.
		// We use a map to store the address reference count for each data item.
		// The key is a type and address pair.
		// The value is a data item with a counter of how many times it has been referenced.
		// If the counter is greater than 1, we know that the data item is a pointer to another data item.
		// We can then encode the pointer as a string and not as an object.
		decoder.dataItems[typeAndAddr{
			irType: uint32(item.Header().Type),
			addr:   item.Header().Address,
		}] = item
	}

	if rootType == nil {
		return probe, errors.New("no root type found")
	}
	var (
		pcs []uint64
		err error
	)
	header, err := event.Event.Header()
	if err != nil {
		return probe, fmt.Errorf("error getting header %w", err)
	}
	// TODO: resolve value from header.Ktime_ns to wall time
	s.Debugger.Snapshot.Timestamp = int(time.Now().UTC().UnixMilli())
	s.Timestamp = s.Debugger.Snapshot.Timestamp

	stackFrames, ok := decoder.stackFrames[header.Stack_hash]
	if !ok {
		pcs, err = event.StackPCs()
		if err != nil {
			return probe, fmt.Errorf("error getting stack pcs %w", err)
		}
		stackFrames, err = symbolicator.Symbolicate(pcs, header.Stack_hash)
		if err != nil {
			return probe, fmt.Errorf("error symbolicating stack %w", err)
		}
		decoder.stackFrames[header.Stack_hash] = stackFrames
	}

	switch where := probe.GetWhere().(type) {
	case ir.FunctionWhere:
		s.Debugger.Snapshot.Probe.Location.Method = where.Location()
		s.Logger.Method = where.Location()
	default:
		return probe, fmt.Errorf(
			"probe %s is not on a supported location: %T",
			probe.GetID(), where,
		)
	}

	s.Logger.Version = probe.GetVersion()
	s.Logger.ThreadID = int(header.Goid)
	s.Debugger.Snapshot.Probe.ID = probe.GetID()
	s.Debugger.Snapshot.Stack.frames = stackFrames
	s.Debugger.Snapshot.Captures.Entry.Arguments = argumentsData{
		event:    event,
		rootType: rootType,
		rootData: s.rootData,
		decoder:  decoder,
	}
	return probe, nil
}
