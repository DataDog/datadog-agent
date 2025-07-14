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
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

type probeEvent struct {
	event *ir.Event
	probe *ir.Probe
}

// Decoder decodes the output of the BPF program into a JSON format.
// It is not guaranteed to be thread-safe.
type Decoder struct {
	program               *ir.Program
	stackFrames           map[uint64][]symbol.StackFrame
	probeEvents           map[ir.TypeID]probeEvent
	snapshotMessage       snapshotMessage
	addressReferenceCount map[typeAndAddr]output.DataItem
}

// NewDecoder creates a new Decoder for the given program.
func NewDecoder(
	program *ir.Program,
) (*Decoder, error) {
	decoder := &Decoder{
		addressReferenceCount: make(map[typeAndAddr]output.DataItem),
		program:               program,
		stackFrames:           make(map[uint64][]symbol.StackFrame),
		probeEvents:           make(map[ir.TypeID]probeEvent),
		snapshotMessage: snapshotMessage{
			DDSource: "dd_debugger",
			Logger: logger{
				Name:   "",
				Method: "",
			},
		},
	}
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			decoder.probeEvents[event.Type.ID] = probeEvent{
				event: event,
				probe: probe,
			}
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
	probe, err = d.snapshotMessage.init(d, event, symbolicator)
	if err != nil {
		return nil, err
	}
	defer d.snapshotMessage.clear()
	err = json.MarshalWrite(out, &d.snapshotMessage)
	return probe, err
}

// Event wraps the output Event from the BPF program. It also adds fields
// that are not present in the BPF program.
type Event struct {
	output.Event
	ServiceName string
}

type snapshotMessage struct {
	Service   string       `json:"service"`
	DDSource  string       `json:"ddsource"`
	Logger    logger       `json:"logger"`
	Debugger  debuggerData `json:"debugger"`
	Timestamp int          `json:"timestamp"`

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

	for item, err := range event.Event.DataItems() {
		if err != nil {
			return nil, fmt.Errorf("error getting data items: %w", err)
		}
		if rootType == nil {
			s.rootData = item.Data()
			var ok bool
			rootType, ok = decoder.program.Types[ir.TypeID(item.Header().Type)].(*ir.EventRootType)
			if !ok {
				return nil, errors.New("expected event of type root first")
			}
			continue
		}
		// We need to keep track of the address reference count for each data item.
		// This is used to avoid infinite recursion when encoding pointers.
		// We use a map to store the address reference count for each data item.
		// The key is a type and address pair.
		// The value is a data item with a counter of how many times it has been referenced.
		// If the counter is greater than 1, we know that the data item is a pointer to another data item.
		// We can then encode the pointer as a string and not as an object.
		decoder.addressReferenceCount[typeAndAddr{
			irType: uint32(item.Header().Type),
			addr:   item.Header().Address,
		}] = item
	}

	if rootType == nil {
		return nil, errors.New("no root type found")
	}
	var (
		pcs []uint64
		err error
	)
	header, err := event.Event.Header()
	if err != nil {
		return nil, fmt.Errorf("error getting header %w", err)
	}
	// TODO: resolve value from header.Ktime_ns to wall time
	s.Debugger.Snapshot.Timestamp = int(time.Now().UTC().UnixMilli())
	s.Timestamp = s.Debugger.Snapshot.Timestamp

	stackFrames, ok := decoder.stackFrames[header.Stack_hash]
	if !ok {
		pcs, err = event.StackPCs()
		if err != nil {
			return nil, fmt.Errorf("error getting stack pcs %w", err)
		}
		stackFrames, err = symbolicator.Symbolicate(pcs, header.Stack_hash)
		if err != nil {
			return nil, fmt.Errorf("error symbolicating stack %w", err)
		}
		decoder.stackFrames[header.Stack_hash] = stackFrames
	}

	probe, ok := decoder.probeEvents[rootType.ID]
	if !ok {
		return nil, fmt.Errorf("error getting probe %w", err)
	}
	switch where := probe.probe.GetWhere().(type) {
	case ir.FunctionWhere:
		s.Debugger.Snapshot.Probe.Location.Method = where.Location()
		s.Logger.Method = where.Location()
	default:
		return nil, errors.New("probe is not on a supported location")
	}

	s.Logger.Version = probe.probe.GetVersion()
	s.Debugger.Snapshot.Probe.ID = probe.probe.GetID()
	s.Debugger.Snapshot.Stack.frames = stackFrames
	s.Debugger.Snapshot.Probe.ID = probe.probe.GetID()
	s.Debugger.Snapshot.Captures.Entry.Arguments = argumentsData{
		event:    event,
		rootType: rootType,
		rootData: s.rootData,
		decoder:  decoder,
	}
	return probe.probe, nil
}

func (s *snapshotMessage) clear() {
	s.Debugger.Snapshot = snapshotData{}
}
