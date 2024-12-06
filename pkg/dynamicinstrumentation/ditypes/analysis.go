// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ditypes

import (
	"debug/dwarf"
	"fmt"
)

// TypeMap contains all the information about functions and their parameters including
// functions that have been inlined in the binary
type TypeMap struct {
	// Functions maps fully-qualified function names to a slice of its parameters
	Functions map[string][]*Parameter

	// InlinedFunctions maps program counters to a slice of dwarf entries used
	// when resolving stack traces that include inlined functions
	InlinedFunctions map[uint64][]*dwarf.Entry

	// FunctionsByPC places DWARF subprogram (function) entries in order by
	// its low program counter which is necessary for resolving stack traces
	FunctionsByPC []*LowPCEntry

	// DeclaredFiles places DWARF compile unit entries in order by its
	// low program counter which is necessary for resolving declared file
	// for the sake of stack traces
	DeclaredFiles []*LowPCEntry
}

// Parameter represents a function parameter as read from DWARF info
type Parameter struct {
	Name                string
	ID                  string
	Type                string
	TotalSize           int64
	Kind                uint
	Location            *Location
	LocationExpressions []LocationExpression
	FieldOffset         uint64
	NotCaptureReason    NotCaptureReason
	ParameterPieces     []*Parameter
}

func (p Parameter) String() string {
	return fmt.Sprintf("%s %s", p.Name, p.Type)
}

// NotCaptureReason is used to convey why a parameter was not captured
type NotCaptureReason uint8

const (
	Unsupported         NotCaptureReason = iota + 1 // Unsupported means the data type of the parameter is unsupported
	NoFieldLocation                                 // NoFieldLocation means the parameter wasn't captured because location information is missing from analysis
	FieldLimitReached                               // FieldLimitReached means the parameter wasn't captured because the data type has too many fields
	CaptureDepthReached                             // CaptureDepthReached means the parameter wasn't captures because the data type has too many levels
)

// SpecialKind is used for clarity in generated events that certain fields weren't read
type SpecialKind uint8

const (
	KindUnsupported         = 255 - iota // KindUnsupported is for unsupported types
	KindCutFieldLimit                    // KindCutFieldLimit is for fields that were cut because of field limit
	KindCaptureDepthReached              // KindCaptureDepthReached is for fields that were cut because of depth limit
)

func (s SpecialKind) String() string {
	switch s {
	case KindUnsupported:
		return "Unsupported"
	case KindCutFieldLimit:
		return "CutFieldLimit"
	default:
		return fmt.Sprintf("%d", s)
	}
}

func (l LocationExpression) String() string {
	return fmt.Sprintf("%s (%d, %d, %d)", l.Opcode.String(), l.Arg1, l.Arg2, l.Arg3)
}

type LocationExpressionOpcode uint

const (
	OpInvalid LocationExpressionOpcode = iota
	OpComment
	OpPrintStatement
	OpReadUserRegister
	OpReadUserStack
	OpReadUserRegisterToOutput
	OpReadUserStackToOutput
	OpDereference
	OpDereferenceToOutput
	OpDereferenceLarge
	OpDereferenceLargeToOutput
	OpDereferenceDynamic
	OpDereferenceDynamicToOutput
	OpReadStringToOutput
	OpApplyOffset
	OpPop
	OpCopy
	OpLabel
	OpSetGlobalLimit
	OpJumpIfGreaterThanLimit
)

func (op LocationExpressionOpcode) String() string {
	switch op {
	case OpInvalid:
		return "Invalid"
	case OpReadUserRegister:
		return "ReadUserRegister"
	case OpReadUserStack:
		return "ReadUserStack"
	case OpReadUserRegisterToOutput:
		return "ReadUserRegisterToOutput"
	case OpReadUserStackToOutput:
		return "ReadUserStackToOutput"
	case OpDereference:
		return "Dereference"
	case OpDereferenceToOutput:
		return "DereferenceToOutput"
	case OpDereferenceLarge:
		return "DereferenceLarge"
	case OpDereferenceLargeToOutput:
		return "DereferenceLargeToOutput"
	case OpDereferenceDynamic:
		return "DereferenceDynamic"
	case OpDereferenceDynamicToOutput:
		return "DereferenceDynamicToOutput"
	case OpApplyOffset:
		return "ApplyOffset"
	case OpPop:
		return "Pop"
	case OpCopy:
		return "Copy"
	default:
		return "Unknown Opcode"
	}
}

func CopyLocationExpression() LocationExpression {
	return LocationExpression{Opcode: OpCopy}
}

func DirectReadLocationExpression(p *Parameter) LocationExpression {
	if p == nil || p.Location == nil {
		return LocationExpression{Opcode: OpInvalid}
	}
	if p.Location.InReg {
		return ReadRegisterLocationExpression(uint(p.Location.Register), uint(p.TotalSize))
	}
	return ReadStackLocationExpression(uint(p.Location.StackOffset), uint(p.TotalSize))
}

func DirectReadToOutputLocationExpression(p *Parameter) LocationExpression {
	if p == nil || p.Location == nil {
		return LocationExpression{Opcode: OpInvalid}
	}
	if p.Location.InReg {
		return ReadRegisterToOutputLocationExpression(uint(p.Location.Register), uint(p.TotalSize))
	}
	return ReadStackToOutputLocationExpression(uint(p.Location.StackOffset), uint(p.TotalSize))
}

// Arg1 = register
// Arg2 = size of element
func ReadRegisterLocationExpression(register, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserRegister, Arg1: register, Arg2: size}
}

// Arg1 = stack offset
// Arg2 = size of element
func ReadStackLocationExpression(offset, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserStack, Arg1: offset, Arg2: size}
}

// Arg1 = register
// Arg2 = size of element
func ReadRegisterToOutputLocationExpression(register, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserRegisterToOutput, Arg1: register, Arg2: size}
}

// Arg1 = stack offset
// Arg2 = size of element
func ReadStackToOutputLocationExpression(offset, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserStackToOutput, Arg1: offset, Arg2: size}
}

// Arg1 = size of value we're reading from the 8 byte address at the top of the stack
func DereferenceLocationExpression(valueSize uint) LocationExpression {
	if valueSize > 8 {
		return LocationExpression{Opcode: OpDereferenceLarge, Arg1: valueSize, Arg2: (valueSize + 7) / 8}
	}
	return LocationExpression{Opcode: OpDereference, Arg1: valueSize}
}

// Arg1 = size of value we're reading from the 8 byte address at the top of the stack
func DereferenceToOutputLocationExpression(valueSize uint) LocationExpression {
	if valueSize > 8 {
		return LocationExpression{Opcode: OpDereferenceLargeToOutput, Arg1: valueSize, Arg2: (valueSize + 7) / 8}
	}
	return LocationExpression{Opcode: OpDereferenceToOutput, Arg1: valueSize}
}

// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
// Arg2 = number of chunks (should be ({{.Arg1}} + 7) / 8)
func DereferenceLargeLocationExpression(typeSize uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceLarge, Arg1: typeSize, Arg2: (typeSize + 7) / 8}
}

// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
// Arg2 = number of chunks (should be ({{.Arg1}} + 7) / 8)
func DereferenceLargeToOutputLocationExpression(typeSize uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceLargeToOutput, Arg1: typeSize, Arg2: (typeSize + 7) / 8}
}

// Maximum limit (Arg1) should be set to the size of each element * max collection length
// Arg1 = maximum limit on bytes read
// Arg2 = number of chunks (should be (max + 7)/8)
// Arg3 = size of each element
func DereferenceDynamicLocationExpression(readLimit, elementSize uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceDynamic, Arg1: readLimit, Arg2: (readLimit + 7) / 8, Arg3: elementSize}
}

// Maximum limit (Arg1) should be set to the size of each element * max collection length
// Arg1 = maximum limit on bytes read
func DereferenceDynamicToOutputLocationExpression(readLimit uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceDynamicToOutput, Arg1: readLimit}
}

// Arg1 = string length limit
func ReadStringToOutput(limit uint16) LocationExpression {
	return LocationExpression{Opcode: OpReadStringToOutput, Arg1: uint(limit)}
}

// Arg1 = uint value (offset) we're adding to the 8-byte address on top of the stack
func ApplyOffsetLocationExpression(offset uint) LocationExpression {
	return LocationExpression{Opcode: OpApplyOffset, Arg1: offset}
}

// Arg1 = number of elements to pop
// Arg2 = size of each element
func PopLocationExpression(numElements, elementSize uint) LocationExpression {
	return LocationExpression{Opcode: OpPop, Arg1: numElements, Arg2: elementSize}
}

// No args, just set label
func InsertLabel(label string) LocationExpression {
	return LocationExpression{Opcode: OpLabel, Label: label}
}

// Arg1 = limit to set
// CollectionIdentifier = the collection that we're limiting
func SetLimitEntry(collectionIdentifier string, limit uint) LocationExpression {
	return LocationExpression{Opcode: OpSetGlobalLimit, CollectionIdentifier: collectionIdentifier, Arg1: limit}
}

// Arg1 = value to compare to global limit variable
// CollectionIdentifier = the collection that we're limiting
// Label = label to jump to if the value is equal to the global limit variable
func JumpToLabelIfEqualToLimit(val uint, collectionIdentifier, label string) LocationExpression {
	return LocationExpression{Opcode: OpJumpIfGreaterThanLimit, CollectionIdentifier: collectionIdentifier, Arg1: val, Label: label}
}

// Label = comment
func InsertComment(comment string) LocationExpression {
	return LocationExpression{Opcode: OpComment, Label: comment}
}

// Label = format
// CollectionIdentifier = arguments
// Example usage: PrintStatement("%d", "variableName")
func PrintStatement(format, arguments string) LocationExpression {
	return LocationExpression{Opcode: OpPrintStatement, Label: format, CollectionIdentifier: arguments}
}

type LocationExpression struct {
	Opcode               LocationExpressionOpcode
	Arg1                 uint
	Arg2                 uint
	Arg3                 uint
	CollectionIdentifier string
	Label                string
}

// Location represents where a particular datatype is found on probe entry
type Location struct {
	InReg            bool
	StackOffset      int64
	Register         int
	NeedsDereference bool
	PointerOffset    uint64
}

func (l Location) String() string {
	return fmt.Sprintf("Location{InReg: %t, StackOffset: %d, Register: %d}", l.InReg, l.StackOffset, l.Register)
}

// LowPCEntry is a helper type used to sort DWARF entries by their low program counter
type LowPCEntry struct {
	LowPC uint64
	Entry *dwarf.Entry
}

// BPFProgram represents a bpf program that's created for a single probe
type BPFProgram struct {
	ProgramText string

	// Used for bpf code generation
	Probe *Probe
}
