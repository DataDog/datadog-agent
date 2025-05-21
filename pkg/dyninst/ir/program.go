// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

import "github.com/DataDog/datadog-agent/pkg/network/go/bininspect"

// ProgramID is a ID corresponding to an instance of a Program.  It is used to
// identify messages from this program as they are communicated over the ring
// buffer.
type ProgramID uint32

// TypeID is a ID corresponding to a type in a program.
type TypeID uint32

// EventID is a ID corresponding to an event output by the program.  It is used
// to identify events as they are communicated over the ring buffer.
type EventID uint32

// Program defines the information needed to generate an eBPF program for a set
// of probes as they apply to a specific binary. This structure is used to drive
// both eBPF program generation and the interpretation of data that comes out of
// the generated program.
type Program struct {
	// ID is the ID of the program. This is used to identify messages from this
	// program as they are communicated over the ring buffer.
	ID ProgramID
	// Probes are the set of probes that comprise the program.
	Probes []*Probe
	// Subprograms are a list of subprograms that will be probed.
	Subprograms []*Subprogram
	// Types are the types that are used in the program.
	Types []Type
	// MaxTypeID is the maximum type ID that has been assigned.
	MaxTypeID TypeID
}

// Subprogram represents a function or method in the program.
type Subprogram struct {
	// Name is the name of the subprogram.
	Name string
	// OutOfLinePCRanges are the ranges of PC values that will be probed for the
	// out-of-line-instances of the subprogram. These are sorted by start PC.
	//
	// What does this mean for inlined subprograms?
	OutOfLinePCRanges []PCRange
	// InlinePCRanges are the ranges of PC values that will be probed for the
	// inlined instances of the subprogram. These are sorted by start PC.
	InlinePCRanges [][]PCRange
	// Variables are the variables that are used in the subprogram.
	Variables []Variable
	// Lines are the lines of the subprogram.
	Lines []SubprogramLine
}

// SubprogramLine represents a line in the subprogram.
type SubprogramLine struct {
	PC              uint64
	File            string
	Line            uint32
	Column          uint32
	IsStatement     bool
	IsPrologueEnd   bool
	IsEpilogueStart bool
}

// Variable represents a variable or parameter in the subprogram.
type Variable struct {
	// Name is the name of the variable.
	Name string
	// Type is the type of the variable.
	Type Type
	// Locations are the locations of the variable in the subprogram.
	Locations []Location
	// IsParameter is true if the variable is a parameter.
	IsParameter bool
}

// Location is the location of a parameter or variable in the subprogram.
type Location struct {
	// PCRange is the range of PC values that will be probed.
	Range PCRange
	// The locations of the pieces of the parameter or variable.
	Location []bininspect.ParameterPiece
}

// PCRange is the range of PC values that will be probed.
type PCRange struct {
	// Start is the start PC value of the range.
	Start uint64
	// End is the end PC value of the range.
	End uint64
}

// Probe represents a probe from the config as it applies to the program.
type Probe struct {
	// The config UUID of the probe.
	ID string
	// The kind of the probe.
	Type ProbeKind
	// The version of the probe.
	Version int
	// Tags that are passed through the probe.
	Tags []string
	// The subprogram to which the probe is attached.
	Subprogram *Subprogram
	// The events that trigger the probe.
	Events []Event
	// Whether the probe should capture a snapshot of the state of the program.
	Snapshot bool
	// TODO: Add template support:
	//	TemplateSegments []TemplateSegment
}

// ProbeKind is the kind of probe.
type ProbeKind uint8

const (
	_ ProbeKind = iota

	// ProbeKindLog is a probe that emits a log.
	ProbeKindLog
	// ProbeKindSpan is a probe that emits a span.
	ProbeKindSpan
	// ProbeKindMetric is a probe that updates a metric.
	ProbeKindMetric
)

// Event corresponds to an action that will occur when a PC is hit.
type Event struct {
	// ID of the event. This is used to identify data produced by the event over
	// the ring buffer.
	ID EventID
	// The datatype of the event.
	Type *EventRootType
	// The PC values at which the event should be injected.
	InjectionPCs []uint64
	// The condition that must be met for the event to be injected.
	Condition *Expression
}
