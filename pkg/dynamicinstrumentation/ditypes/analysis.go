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

// TypeMap contains all the information about functions and their parameters
type TypeMap struct {
	// Functions maps fully-qualified function names to a slice of its parameters
	Functions map[string][]*Parameter

	// FunctionsByPC places DWARF subprogram (function) entries in order by
	// its low program counter which is necessary for resolving stack traces
	FunctionsByPC []*FuncByPCEntry

	// DeclaredFiles places DWARF compile unit entries in order by its
	// low program counter which is necessary for resolving declared file
	// for the sake of stack traces
	DeclaredFiles []*DwarfFilesEntry
}

// Parameter represents a function parameter as read from DWARF info
type Parameter struct {
	Name                string               // Name is populated by the local name of the parameter
	ID                  string               // ID is randomly generated for each parameter to avoid
	Type                string               // Type is a string representation of the type name
	TotalSize           int64                // TotalSize is the size of the parameter type
	Kind                uint                 // Kind is a constant representation of the type, see reflect.Kind
	Location            *Location            // Location represents where the parameter will be in memory when passed to the target function
	LocationExpressions []LocationExpression // LocationExpressions are the needed instructions for extracting the parameter value from memory
	FieldOffset         uint64               // FieldOffset is the offset of Parameter field within a struct, if it is a struct field
	DoNotCapture        bool                 // DoNotCapture signals to code generation that this parameter or field shouldn't have it's value captured
	NotCaptureReason    NotCaptureReason     // NotCaptureReason conveys to the user why the parameter was not captured
	ParameterPieces     []*Parameter         // ParameterPieces are the sub-fields, such as struct fields or array elements
}

func (p Parameter) String() string {
	return fmt.Sprintf("%s %s", p.Name, p.Type)
}

// NotCaptureReason is used to convey why a parameter was not captured
type NotCaptureReason uint8

const (
	Unsupported            NotCaptureReason = iota + 1 // Unsupported means the data type of the parameter is unsupported
	NoFieldLocation                                    // NoFieldLocation means the parameter wasn't captured because location information is missing from analysis
	FieldLimitReached                                  // FieldLimitReached means the parameter wasn't captured because the data type has too many fields
	CaptureDepthReached                                // CaptureDepthReached means the parameter wasn't captures because the data type has too many levels
	CollectionLimitReached                             // CollectionLimitReached means the parameter wasn't captured because the data type has too many elements
)

func (r NotCaptureReason) String() string {
	switch r {
	case Unsupported:
		return "unsupported"
	case NoFieldLocation:
		return "no field location"
	case FieldLimitReached:
		return "fieldCount"
	case CaptureDepthReached:
		return "depth"
	case CollectionLimitReached:
		return "collectionSize"
	default:
		return fmt.Sprintf("unknown reason (%d)", r)
	}
}

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
	case KindCaptureDepthReached:
		return "CaptureDepthReached"
	default:
		return fmt.Sprintf("%d", s)
	}
}

func (l LocationExpression) String() string {
	return fmt.Sprintf("Opcode: %s Args: [%d, %d, %d] Label: %s Collection ID: %s\n",
		l.Opcode.String(),
		l.Arg1,
		l.Arg2,
		l.Arg3,
		l.Label,
		l.CollectionIdentifier)
}

// LocationExpressionOpcode uniquely identifies each location expression operation
type LocationExpressionOpcode uint

const (
	// OpInvalid represents an invalid operation
	OpInvalid LocationExpressionOpcode = iota
	// OpComment represents a comment operation
	OpComment
	// OpPrintStatement represents a print statement operation
	OpPrintStatement
	// OpReadUserRegister represents an operation to read a user register
	OpReadUserRegister
	// OpReadUserStack represents an operation to read the user stack
	OpReadUserStack
	// OpReadUserRegisterToOutput represents an operation to read a user register and output the value
	OpReadUserRegisterToOutput
	// OpReadUserStackToOutput represents an operation to read the user stack and output the value
	OpReadUserStackToOutput
	// OpDereference represents an operation to dereference a pointer
	OpDereference
	// OpDereferenceToOutput represents an operation to dereference a pointer and output the value
	OpDereferenceToOutput
	// OpDereferenceLarge represents an operation to dereference a large pointer
	OpDereferenceLarge
	// OpDereferenceLargeToOutput represents an operation to dereference a large pointer and output the value
	OpDereferenceLargeToOutput
	// OpDereferenceDynamic represents an operation to dynamically dereference a pointer
	OpDereferenceDynamic
	// OpDereferenceDynamicToOutput represents an operation to dynamically dereference a pointer and output the value
	OpDereferenceDynamicToOutput
	// OpReadStringToOutput represents an operation to read a string and output the value
	OpReadStringToOutput
	// OpApplyOffset represents an operation to apply an offset
	OpApplyOffset
	// OpPop represents an operation to pop a value from the stack
	OpPop
	// OpCopy represents an operation to copy a value
	OpCopy
	// OpLabel represents a label operation
	OpLabel
	// OpSetGlobalLimit represents an operation to set a global limit
	OpSetGlobalLimit
	// OpJumpIfGreaterThanLimit represents an operation to jump if a value is greater than a limit
	OpJumpIfGreaterThanLimit
	// OpPopPointerAddress is a special opcode for a compound operation (combination of location expressions)
	// that are used for popping the address when reading pointers
	OpPopPointerAddress
	// OpSetParameterIndex sets the parameter index in the base event's param_indicies array field
	OpSetParameterIndex
)

func (op LocationExpressionOpcode) String() string {
	switch op {
	case OpInvalid:
		return "Invalid"
	case OpComment:
		return "Comment"
	case OpPrintStatement:
		return "PrintStatement"
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
	case OpReadStringToOutput:
		return "ReadStringToOutput"
	case OpApplyOffset:
		return "ApplyOffset"
	case OpPop:
		return "Pop"
	case OpCopy:
		return "Copy"
	case OpLabel:
		return "Label"
	case OpSetGlobalLimit:
		return "SetGlobalLimit"
	case OpJumpIfGreaterThanLimit:
		return "JumpIfGreaterThanLimit"
	case OpSetParameterIndex:
		return "SetParamIndex"
	default:
		return fmt.Sprintf("LocationExpressionOpcode(%d)", int(op))
	}
}

// CopyLocationExpression express creates an expression which
// duplicates the u64 element on the top of the BPF parameter stack.
func CopyLocationExpression() LocationExpression {
	return LocationExpression{Opcode: OpCopy}
}

// PopPointerAddressCompoundLocationExpression is a compound location
// expression, meaning it's a combination of expressions with the
// specific purpose of popping the address for pointer values
func PopPointerAddressCompoundLocationExpression() LocationExpression {
	return LocationExpression{
		Opcode: OpPopPointerAddress,
		IncludedExpressions: []LocationExpression{
			CopyLocationExpression(),
			PopLocationExpression(1, 8),
		},
	}
}

// DirectReadLocationExpression creates an expression which
// directly reads a value from either a specific register or stack offset
// and writes it to the bpf param stack
func DirectReadLocationExpression(p *Parameter) LocationExpression {
	if p == nil || p.Location == nil {
		return LocationExpression{Opcode: OpInvalid}
	}
	if p.Location.InReg {
		return ReadRegisterLocationExpression(uint(p.Location.Register), uint(p.TotalSize))
	}
	return ReadStackLocationExpression(uint(p.Location.StackOffset), uint(p.TotalSize))
}

// DirectReadToOutputLocationExpression creates an expression which
// directly reads a value from either a specific register or stack offset
// and writes it to the output buffer
func DirectReadToOutputLocationExpression(p *Parameter) LocationExpression {
	if p == nil || p.Location == nil {
		return LocationExpression{Opcode: OpInvalid}
	}
	if p.Location.InReg {
		return ReadRegisterToOutputLocationExpression(uint(p.Location.Register), uint(p.TotalSize))
	}
	return ReadStackToOutputLocationExpression(uint(p.Location.StackOffset), uint(p.TotalSize))
}

// ReadRegisterLocationExpression creates an expression which
// reads `size` bytes from register `reg` into a u64 which is then pushed to
// the top of the BPF parameter stack.
// Arg1 = register
// Arg2 = size of element
func ReadRegisterLocationExpression(register, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserRegister, Arg1: register, Arg2: size}
}

// ReadStackLocationExpression creates an expression which
// reads `size` bytes from the traced program's stack at offset `stack_offset`
// into a u64 which is then pushed to the top of the BPF parameter stack.
// Arg1 = stack offset
// Arg2 = size of element
func ReadStackLocationExpression(offset, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserStack, Arg1: offset, Arg2: size}
}

// ReadRegisterToOutputLocationExpression creates an expression which
// reads `size` bytes from register `reg` into a u64 which is then written to
// the output buffer.
// Arg1 = register
// Arg2 = size of element
func ReadRegisterToOutputLocationExpression(register, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserRegisterToOutput, Arg1: register, Arg2: size}
}

// ReadStackToOutputLocationExpression creates an expression which
// reads `size` bytes from the traced program's stack at offset `stack_offset`
// into a u64 which is then written to the output buffer
// Arg1 = stack offset
// Arg2 = size of element
func ReadStackToOutputLocationExpression(offset, size uint) LocationExpression {
	return LocationExpression{Opcode: OpReadUserStackToOutput, Arg1: offset, Arg2: size}
}

// DereferenceLocationExpression creates an expression which
// pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `valueSize` from it, and pushes that value (encoded as a u64)
// back to the BPF parameter stack.
// It should only be used for types of 8 bytes or less
// Arg1 = size of value we're reading from the 8 byte address at the top of the stack
func DereferenceLocationExpression(valueSize uint) LocationExpression {
	if valueSize > 8 {
		return LocationExpression{Opcode: OpDereferenceLarge, Arg1: valueSize, Arg2: (valueSize + 7) / 8}
	}
	return LocationExpression{Opcode: OpDereference, Arg1: valueSize}
}

// DereferenceToOutputLocationExpression creates an expression which
// pops the 8-byte address from the top of the BPF parameter stack and
// dereferences it, reading a value of size `valueSize` from it, and writes that value
// directly to the output buffer.
// It should only be used for types of 8 bytes or less
// Arg1 = size of value we're reading from the 8 byte address at the top of the stack
func DereferenceToOutputLocationExpression(valueSize uint) LocationExpression {
	if valueSize > 8 {
		return LocationExpression{Opcode: OpDereferenceLargeToOutput, Arg1: valueSize, Arg2: (valueSize + 7) / 8}
	}
	return LocationExpression{Opcode: OpDereferenceToOutput, Arg1: valueSize}
}

// DereferenceLargeLocationExpression creates an expression which
// pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `typeSize` from it, and pushes that value, encoded in 8-byte chunks
// to the BPF parameter stack. This is safe to use for types larger than 8-bytes.
// back to the BPF parameter stack.
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
// Arg2 = number of chunks (should be ({{.Arg1}} + 7) / 8)
func DereferenceLargeLocationExpression(typeSize uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceLarge, Arg1: typeSize, Arg2: (typeSize + 7) / 8}
}

// DereferenceLargeToOutputLocationExpression creates an expression which
// pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `typeSize` from it, and writes that value to the output buffer.
// This is safe to use for types larger than 8-bytes.
// Arg1 = size in bytes of value we're reading from the 8 byte address at the top of the stack
// Arg2 = number of chunks (should be ({{.Arg1}} + 7) / 8)
func DereferenceLargeToOutputLocationExpression(typeSize uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceLargeToOutput, Arg1: typeSize, Arg2: (typeSize + 7) / 8}
}

// DereferenceDynamicToOutputLocationExpression creates an expression which
// reads an 8-byte length from the top of the BPF parameter stack, followed by
// an 8-byte address. It applies the maximum `readLimit` to the length, then dereferences the address to
// the output buffer.
// Maximum limit (Arg1) should be set to the size of each element * max collection length
// Arg1 = maximum limit on bytes read
func DereferenceDynamicToOutputLocationExpression(readLimit uint) LocationExpression {
	return LocationExpression{Opcode: OpDereferenceDynamicToOutput, Arg1: readLimit}
}

// ReadStringToOutputLocationExpression creates an expression which
// reads a Go string to the output buffer, limited in length by `limit`.
// In Go, strings are internally implemented as structs with two fields. The fields are length,
// and a pointer to a character array. This expression expects the address of the string struct
// itself to be on the top of the stack.
// Arg1 = string length limit
func ReadStringToOutputLocationExpression(limit uint16) LocationExpression {
	return LocationExpression{Opcode: OpReadStringToOutput, Arg1: uint(limit)}
}

// ApplyOffsetLocationExpression creates an expression which
// adds `offset` to the 8-byte address on the top of the bpf parameter stack.
// Arg1 = uint value (offset) we're adding to the 8-byte address on top of the stack
func ApplyOffsetLocationExpression(offset uint) LocationExpression {
	return LocationExpression{Opcode: OpApplyOffset, Arg1: offset}
}

// PopLocationExpression creates an expression which
// writes to output `num_elements` elements, each of size `elementSize, from the top of the stack.
// Arg1 = number of elements to pop
// Arg2 = size of each element
func PopLocationExpression(numElements, elementSize uint) LocationExpression {
	return LocationExpression{Opcode: OpPop, Arg1: numElements, Arg2: elementSize}
}

// InsertLabel inserts a label in the bpf program
// No args, just set label
func InsertLabel(label string) LocationExpression {
	return LocationExpression{Opcode: OpLabel, Label: label}
}

// SetLimitEntry associates a collection identifier with the passed limit
// Arg1 = limit to set
// CollectionIdentifier = the collection that we're limiting
func SetLimitEntry(collectionIdentifier string, limit uint) LocationExpression {
	return LocationExpression{Opcode: OpSetGlobalLimit, CollectionIdentifier: collectionIdentifier, Arg1: limit}
}

// JumpToLabelIfEqualToLimit jumps to a specified label if the limit associated with the collection (by identifier)
// is equal to the passed value
// Arg1 = value to compare to global limit variable
// CollectionIdentifier = the collection that we're limiting
// Label = label to jump to if the value is equal to the global limit variable
func JumpToLabelIfEqualToLimit(val uint, collectionIdentifier, label string) LocationExpression {
	return LocationExpression{Opcode: OpJumpIfGreaterThanLimit, CollectionIdentifier: collectionIdentifier, Arg1: val, Label: label}
}

// InsertComment inserts a comment into the bpf program
// Label = comment
func InsertComment(comment string) LocationExpression {
	return LocationExpression{Opcode: OpComment, Label: comment}
}

// PrintStatement inserts a print statement into the bpf program
// Label = format
// CollectionIdentifier = arguments
// Example usage: PrintStatement("%d", "variableName")
func PrintStatement(format, arguments string) LocationExpression {
	return LocationExpression{Opcode: OpPrintStatement, Label: format, CollectionIdentifier: arguments}
}

// SetParameterIndexLocationExpression creates an expression which
// sets the parameter index in the base event's param_indicies array field.
// This allows tracking which parameters were successfully collected.
// Arg1 = index of the parameter
func SetParameterIndexLocationExpression(index uint16) LocationExpression {
	return LocationExpression{
		Opcode: OpSetParameterIndex,
		Arg1:   uint(index),
	}
}

// LocationExpression is an operation which will be executed in bpf with the purpose
// of capturing parameters from a running Go program
type LocationExpression struct {
	Opcode               LocationExpressionOpcode
	Arg1                 uint
	Arg2                 uint
	Arg3                 uint
	CollectionIdentifier string
	Label                string
	IncludedExpressions  []LocationExpression
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

// DwarfFilesEntry represents the list of files used in a DWARF compile unit
type DwarfFilesEntry struct {
	LowPC uint64
	Files []*dwarf.LineFile
}

// BPFProgram represents a bpf program that's created for a single probe
type BPFProgram struct {
	ProgramText string

	// Used for bpf code generation
	Probe *Probe
}

//nolint:all
func PrintLocationExpressions(expressions []LocationExpression) {
	for i := range expressions {
		fmt.Println(expressions[i].String())
	}
}

// FuncByPCEntry represents useful data associated with a function entry in DWARF
type FuncByPCEntry struct {
	LowPC      uint64
	Fn         string
	FileNumber int64
	Line       int64
}

// RemoteConfigCallback is the name of the function in dd-trace-go which we hook for retrieving
// probe configurations
const RemoteConfigCallback = "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.passProbeConfiguration"
