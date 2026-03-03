// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
)

// ProgramID is a ID corresponding to an instance of a Program.  It is used to
// identify messages from this program as they are communicated over the ring
// buffer.
type ProgramID uint32

// TypeID is a ID corresponding to a type in a program.
type TypeID uint32

// SubprogramID is a ID corresponding to a subprogram in a program.
type SubprogramID uint32

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
	Types map[TypeID]Type
	// MaxTypeID is the maximum type ID that has been assigned.
	MaxTypeID TypeID
	// Issues is a list of probes that could not be created.
	Issues []ProbeIssue
	// GoModuledataInfo is used to resolve types from interfaces.
	GoModuledataInfo GoModuledataInfo
	// CommonTypes store references to common types.
	CommonTypes CommonTypes
}

// GoModuledataInfo is information about the runtime-internal structure used to
// translate type pointer addresses to Go runtime type IDs. This information is
// used in the generated program to resolve type information for interface
// values.
type GoModuledataInfo struct {
	// FirstModuledataAddr is the virtual memory address of the firstmoduledata
	// variable.
	//
	// See https://github.com/golang/go/blob/5a56d884/src/runtime/symtab.go#L483
	FirstModuledataAddr uint64
	// TypesOffset is the offset in the runtime.moduledata type of
	// the types field.
	//
	// See https://github.com/golang/go/blob/5a56d884/src/runtime/symtab.go#L414
	TypesOffset uint32
}

// CommonTypes stores references to common types.
type CommonTypes struct {
	// G corresponds to runtime.g, non-nil
	G *StructureType
	// M corresponds to runtime.m, non-nil
	M *StructureType
}

// InlinePCRanges represent the pc ranges for a single instance of an inlined subprogram.
// Ranges correspond to the inlined instance itself. RootRanges correspond to the pc ranges
// of a subprogram at the root of the tree formed by inlined subroutines. E.g. if this is a
// subprogram A, that has been inlined into subprogram B, and subprogram B has been inlined
// to a subprogram C, and C is not inlined, then these are pc ranges of C.
type InlinePCRanges struct {
	// Non-overlapping and sorted.
	Ranges []PCRange
	// Non-overlapping and sorted.
	RootRanges []PCRange
}

// Subprogram represents a function or method in the program.
type Subprogram struct {
	// ID is the ID of the subprogram.
	ID SubprogramID
	// Name is the name of the subprogram.
	Name string
	// OutOfLinePCRanges are the ranges of PC values that will be probed for the
	// out-of-line instance of the subprogram. These are sorted by start PC.
	// Some functions may be inlined only in certain callers, in which case
	// both OutOfLinePCRanges and InlinedPCRanges will be non-empty.
	OutOfLinePCRanges []PCRange
	// InlinePCRanges are the ranges of PC values that will be probed for the
	// inlined instances of the subprogram. These are sorted by start PC.
	InlinePCRanges []InlinePCRanges
	// Variables are the variables that are used in the subprogram.
	Variables []*Variable
}

// VariableRole is the role of a variable within a subprogram.
type VariableRole uint8

// VariableRole values.
const (
	_ VariableRole = iota
	VariableRoleParameter
	VariableRoleReturn
	VariableRoleLocal
)

func (vr VariableRole) String() string {
	switch vr {
	case VariableRoleParameter:
		return "Parameter"
	case VariableRoleReturn:
		return "Return"
	case VariableRoleLocal:
		return "Local"
	default:
		return fmt.Sprintf("VariableRole(%d)", vr)
	}
}

// Variable represents a variable or parameter in the subprogram.
type Variable struct {
	// Name is the name of the variable.
	Name string
	// Type is the type of the variable.
	Type Type
	// Locations are the locations of the variable in the subprogram.
	// Sorted by low limit of their ranges. Note the ranges might overlap,
	// in case of variables inlined multiple times in the same parent subprogram.
	Locations []Location
	// Role is the role of the variable within the subprogram.
	Role VariableRole
}

// PCRange is the range of PC values that will be probed.
type PCRange = [2]uint64

// Template represents the concrete template structure for a probe.
type Template struct {
	// TemplateString is the complete template string.
	TemplateString string
	// Segments are the ordered parts of the template.
	Segments []TemplateSegment
}

// TemplateSegment represents a concrete part of the template.
type TemplateSegment interface {
	templateSegment() // marker method
}

// StringSegment is a string literal in the template.
type StringSegment string

func (s StringSegment) templateSegment() {}

// JSONSegment is an expression segment in the template.
type JSONSegment struct {
	// JSON is the AST of the DSL segment.
	JSON exprlang.Expr
	// DSL is the raw expression language segment.
	DSL string
	// EventKind is the kind of the event within the probe that corresponds to this segment (i.e. entry, return, line).
	EventKind EventKind
	// EventExpressionIndex is the index of the expression within the event.
	EventExpressionIndex int
}

func (s *JSONSegment) templateSegment() {}

// InvalidSegment is a segment that represents an issue with the template.
type InvalidSegment struct {
	// Error is the error that occurred while parsing the segment.
	Error string
	DSL   string
}

func (s InvalidSegment) templateSegment() {}

// DurationSegment is a segment that is a simple reference to @duration.
type DurationSegment struct{}

func (s *DurationSegment) templateSegment() {}

// Probe represents a probe from the config as it applies to the program.
type Probe struct {
	ProbeDefinition
	// The subprogram to which the probe is attached.
	Subprogram *Subprogram
	// The events that trigger the probe.
	Events []*Event
	// Template contains the concrete template structure for this probe.
	Template *Template
}

// Event corresponds to an action that will occur when a PC is hit.
type Event struct {
	// Kind is the kind of event.
	Kind EventKind
	// SourceLine for line events, empty otherwise.
	SourceLine string `json:"-"`
	// The datatype of the event.
	Type *EventRootType
	// The PC values at which the event should be injected. Sorted by PC.
	InjectionPoints []InjectionPoint
	// The condition that must be met for the event to be injected.
	Condition *Expression
}

// InjectionPoint is a point at which an event should be injected.
type InjectionPoint struct {
	// The PC value at which the event should be injected.
	PC uint64
	// Whether the function at that PC is frameless.
	Frameless bool
	// HasAssociatedReturn is true if there is going to be a return associated
	// with this call.
	HasAssociatedReturn bool `json:"-"`
	// NoReturnReason is the reason why there is no return associated with this
	// call. Only set if HasAssociatedReturn is false.
	NoReturnReason NoReturnReason `json:"-"`
	// TopPCOffset is the offset of the top PC from the entry PC.
	TopPCOffset int8 `json:"-"`
}

// This must be kept in sync with the no_return_reason enum in the ebpf/types.h
// file.
type NoReturnReason uint8

const (
	NoReturnReasonNone            NoReturnReason = 0
	NoReturnReasonReturnsDisabled NoReturnReason = 1
	NoReturnReasonLineProbe       NoReturnReason = 2
	NoReturnReasonInlined         NoReturnReason = 3
	NoReturnReasonNoBody          NoReturnReason = 4
	NoReturnReasonIsReturn        NoReturnReason = 5
)
