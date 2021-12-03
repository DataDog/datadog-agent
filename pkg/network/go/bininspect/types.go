package bininspect

import (
	"reflect"

	"github.com/go-delve/delve/pkg/goversion"
)

// Config contains options for controlling what is included
// in the results of the binary inspection process,
// such as the struct offsets to obtain
// and the functions to examine.
type Config struct {
	Functions         []FunctionConfig
	StructOffsets     []StructOffsetConfig
	StaticItabEntries []StaticItabEntryConfig
}

// Result is the result of the binary inspection process.
type Result struct {
	Arch                 GoArch
	ABI                  GoABI
	GoVersion            goversion.GoVersion
	IncludesDebugSymbols bool
	GoroutineIDMetadata  GoroutineIDMetadata
	Functions            []FunctionMetadata
	// If IncludesDebugSymbols is false, then this slice will be nil/empty
	// (regardless of the struct offset configs).
	StructOffsets []StructOffset
	// If IncludesDebugSymbols is false, then this slice will be nil/empty
	// (regardless of the static itab entry configs).
	StaticItabEntries []StaticItabEntry
}

// GoArch only includes supported architectures,
// and the underlying values are compatible with GOARCH values.
type GoArch string

const (
	// GoArchX86_64 corresponds to x86 64-bit ("amd64")
	GoArchX86_64 GoArch = "amd64"
	// GoArchARM64 corresponds to ARM 64-bit ("arm64")
	GoArchARM64 GoArch = "arm64"
)

// GoroutineIDMetadata contains information
// that can be used to reliably determine the ID
// of the currently-running goroutine from an eBPF uprobe.
type GoroutineIDMetadata struct {
	// The offset of the `goid` field within `runtime.g`
	GoroutineIDOffset uint64
	// Whether the pointer to the current `runtime.g` value is in a register.
	// If true, then `RuntimeGRegister` is given.
	// Otherwise, `RuntimeGTLSAddrOffset` is given
	RuntimeGInRegister bool

	// The register that the pointer to the current `runtime.g` is in.
	RuntimeGRegister int
	// The offset of the `runtime.g` value within thread-local-storage.
	RuntimeGTLSAddrOffset uint64
}

// PointerSize gets the size, in bytes, of pointers on the architecture
func (a *GoArch) PointerSize() uint {
	switch *a {
	case GoArchX86_64:
		return 8
	case GoArchARM64:
		return 8
	default:
		return 0
	}
}

// GoABI is the type of ABI used by the Go compiler when generating a binary
type GoABI string

const (
	// GoABIStack is the same as abi0
	GoABIStack GoABI = "stack"
	// GoABIRegister is the same as abi internal (as of 2021-11-12)
	GoABIRegister GoABI = "register"
)

// FunctionConfig controls the extraction of information about a single function
// that can be used to attach an eBPF uprobe to.
type FunctionConfig struct {
	// The full name of the function to gather metadata for.
	// ex: crypto/tls.(*Conn).Write
	Name string
	// Whether to gather the locations of the return instructions in the function.
	IncludeReturnLocations bool
}

// FunctionMetadata contains the results of the inspection process that
// that can be used to attach an eBPF uprobe to a single function.
type FunctionMetadata struct {
	// The full name of the function that this metadata is for.
	// ex: crypto/tls.(*Conn).Write
	Name string
	// The virtual address for the function's entrypoint for non-PIE's,
	// or offset to the file load address for PIE's.
	//
	// This should really probably be the location of the end of the prologue
	// (which might help with parameter locations being half-spilled),
	// but so far using the first PC position in the function has worked
	// for the functions we're tracing.
	// See:
	// - https://github.com/go-delve/delve/pull/2704#issuecomment-944374511
	//   (which implies that the instructions in the prologue
	//   might get executed multiple times over the course of a single function call,
	//   though I'm not sure under what circumstances this might be true)
	EntryLocation uint64
	// A list of parameter metadata structs
	// used for extracting the function arguments in the eBPF probe.
	//
	// When optimizations are enabled, and depending on the Go version,
	// it has been observed that these parameter locations are sometimes missing pieces
	// (either due to them being eliminated on purpose
	// (as in the case of an unused slice length/capacity/data) being omitted)
	// or due to (spooky behavior in the compiler?))
	// or missing entire parameters (again due to (spooky behavior in the compiler?)).
	// This is unfortunate; I don't think there is a clean way to work around this
	// (since the data is just missing).
	// The best solution I have found is to:
	// - manually handle cases where entire parameters are missing
	//   if it's known that they only occur on a specific Go version
	//   (Go 1.16.* is one that I have found to be troublesome)
	//   and insert the appropriate parameter locations if known.
	// - if all of the parameters are missing,
	//   then I have found that manually re-implementing
	//   the stack/register allocation algorithm for function arguments/returns
	//   (depending on the ABI, and using the sizes of the values from the type metadata)
	//   can work to still obtain the parameters at runtime.
	//   This is probably much more brittle
	//   than getting the locations directly from the debug information.
	//
	// If the outer result's IncludesDebugSymbols is false,
	// then this slice always will be nil/empty.
	Parameters []ParameterMetadata
	// A list of locations for each return instruction in the compiled function machine code.
	// Each element is the virtual address for the function's entrypoint for non-PIE's,
	// or offset to the file load address for PIE's.
	// Only given if `IncludesReturn` is true.
	//
	// Note that this may not behave well with panics or defer statements.
	ReturnLocations []uint64
}

// ParameterMetadata contains information about a single parameter
// (both traditional input parameters and return "parameters").
// This includes both data about the type of the parameter
// (and its total size, in bytes)
// as well as the position of the parameter (or its pieces)
// at runtime, which will either be on the stack or in registers.
//
// See the note on FunctionMetadata.Parameters
// for limitations of the position data included within this struct.
//
// Note that while Go only ever passes parameters
// entirely in the stack or entirely in registers
// (depending on the ABI used and the version of the Go compiler),
// it has been observed that the position metadata emitted by the compiler
// at the function's entrypoint indicates partially-spilled parameters
// that have some pieces still in registers
// and some pieces that have been moved into their reserved space on the stack.
// It's a little unclear whether the location metadata is *correct* in these cases
// (i.e. when attaching an eBPF uprobe at the entrypoint of the function,
// has the first instruction to spill the registers actually been executed?),
// but so far, taking the locations at face value and interpreting them directly
// (specifically handling mixed stack/register locations)
// has been successful in getting the expected values from eBPF.
//
// TODO: look into cases where a middle piece of a parameter has been eliminated
//       (such as the length of a slice),
//       and make sure result is expected/handled well.
//       Then, document such cases.
type ParameterMetadata struct {
	// Total size in bytes.
	TotalSize int64
	// Kind of variable.
	Kind reflect.Kind

	// Pieces that make up the location of this parameter at runtime
	Pieces []ParameterPiece
}

// ParameterPiece represents a single piece of a parameter
// that either exists entirely on the stack or in a register.
// This might be the full parameter, or might be just part of it
// (especially in the case of slices/interfaces/strings
// that are made up of multiple word-sized parts).
type ParameterPiece struct {
	// Size of this piece in bytes
	Size int64
	// True if this piece is contained in a register.
	InReg bool
	// Offset from the stackpointer.
	// Only given if the piece resides on the stack.
	StackOffset int64
	// Register number of the piece.
	// Only given if the piece resides in registers.
	Register int
}

// StructOffsetConfig controls the extraction
// of a single struct field's offset from the binary's debug information.
type StructOffsetConfig struct {
	// Fully-qualified name of the struct
	// Ex.
	// - `crypto/tls.Conn`
	// - `internal/poll.FD`
	// - `net.TCPConn`
	// - `main.Foo`
	// - `github.com/user/repo/cmd/test-cmd/foo.Bar`
	StructName string
	// Name of the field in the struct
	FieldName string
}

// StructOffset contains the result of a single struct field's offset
// (including the original struct/field it is an offset for).
type StructOffset struct {
	// Fully-qualified name of the struct
	StructName string
	// Name of the field in the struct
	FieldName string
	// Offset (in bytes) of the field from the struct's address
	Offset uint64
}

// StaticItabEntryConfig controls the extraction
// of a single (struct, interface) pair that are expected to have a static i-table
// (interface table) entry in the binary being inspected.
// I-table entries associate a specific struct and interface together,
// and are pointed to by a pointer in `interface{}` values at runtime.
// As such, they are useful to resolve the concrete type of `interface{}` values from eBPF.
// In general, i-table entries are either created "statically" (during compilation)
// or "dynamically" (at runtime, as types are cast).
// By definition, dynamically-created i-table entries
// cannot be statically extracted from a binary,
// so it is only possible to get the corresponding i-table entry
// if it was statically created at compile time.
//
// The docs in runtime/iface.go describe when these i-table entries are generated, namely:
// > [the] compiler statically generates itab's for all interface/type pairs
// > used in switches (which are added to itabTable in itabsinit)
// (source: https://github.com/golang/go/blob/36be0beb05043e8ef3b8d108e9f8977b5eac0c87/src/runtime/iface.go#L70-L73)
//
// More information:
// - https://github.com/teh-cmc/go-internals/blob/master/chapter2_interfaces/README.md
// - https://github.com/golang/go/blob/36be0beb05043e8ef3b8d108e9f8977b5eac0c87/src/runtime/iface.go
// - https://github.com/golang/go/blob/c5c1955077cb94736b0f311b3a02419d166f45ac/src/cmd/compile/internal/reflectdata/reflect.go#L1282
//
// Note that if a binary is missing its ELF symbols
// (likely due to being stripped at some point),
// the static itab entries can't be resolved using the current method.
// It seems like the Go compiler embeds itab information in its "symtab" data:
// https://github.com/golang/go/blob/c5c1955077cb94736b0f311b3a02419d166f45ac/src/runtime/symtab.go#L437
// so there might be another way of resolving the static itab entries,
// even on a stripped binary.
type StaticItabEntryConfig struct {
	// Fully-qualified name of the struct,
	// with an `*` in front if the struct implements the interface
	// as a pointer receiver
	StructName string
	// Fully-qualified name of the interface
	InterfaceName string
}

// StaticItabEntry conatains the result of finding a single static i-table entry
// (including the original struct/interface it is an entry for).
type StaticItabEntry struct {
	// Fully-qualified name of the struct,
	// with an `*` in front if the struct implements the interface
	// as a pointer receiver
	StructName string
	// Fully-qualified name of the interface
	InterfaceName string
	// The value of `runtime.iface.tab` that `interface{}` values will have
	// when they match the given struct + interface pair
	EntryIndex uint64
}
