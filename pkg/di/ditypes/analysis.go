package ditypes

import (
	"debug/dwarf"
	"fmt"
)

// TypeMap contains all the information about functions and their parameters including
// functions that have been inlined in the binary
type TypeMap struct {
	// Functions maps fully-qualified function names to a slice of its parameters
	Functions map[string][]Parameter

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

type Parameter struct {
	Name             string
	ID               string
	Type             string
	TotalSize        int64
	Kind             uint
	Location         Location
	NotCaptureReason NotCaptureReason
	ParameterPieces  []Parameter
}

func (p Parameter) String() string {
	return fmt.Sprintf("%s %s", p.Name, p.Type)
}

type NotCaptureReason uint8

const (
	Unsupported NotCaptureReason = iota + 1
	FieldLimitReached
	CaptureDepthReached
)

type SpecialKind uint8

const (
	KindUnsupported = 255 - iota
	KindCutFieldLimit
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

type LowPCEntry struct {
	LowPC uint64
	Entry *dwarf.Entry
}
