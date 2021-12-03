// +build linux_bpf

package gotls

import (
	"debug/elf"
	"fmt"
	"reflect"

	"github.com/go-delve/delve/pkg/goversion"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/http/gotls/lookup"
)

type tracedFunction string

const (
	goTlsConnWrite tracedFunction = "crypto/tls.(*Conn).Write"
	goTlsConnRead  tracedFunction = "crypto/tls.(*Conn).Read"
	goTlsConnClose tracedFunction = "crypto/tls.(*Conn).Close"
)

type attachmentArgs struct {
	writeAddress        uint64
	readAddress         uint64
	readReturnAddresses []uint64
	closeAddress        uint64
	// This struct gets passed to the EBPF probe
	probeData ebpf.TlsProbeData
}

var inspectConfig = bininspect.Config{
	Functions: []bininspect.FunctionConfig{
		{
			Name:                   string(goTlsConnWrite),
			IncludeReturnLocations: false,
		},
		{
			Name:                   string(goTlsConnRead),
			IncludeReturnLocations: true,
		},
		{
			Name:                   string(goTlsConnClose),
			IncludeReturnLocations: false,
		},
	},
	StructOffsets: []bininspect.StructOffsetConfig{
		{
			StructName: "crypto/tls.Conn",
			FieldName:  "conn",
		},
		{
			StructName: "net.TCPConn",
			FieldName:  "conn",
		},
		{
			StructName: "net.conn",
			FieldName:  "fd",
		},
		{
			StructName: "net.netFD",
			FieldName:  "pfd",
		},
		{
			StructName: "internal/poll.FD",
			FieldName:  "Sysfd",
		},
	},
	StaticItabEntries: []bininspect.StaticItabEntryConfig{
		{
			StructName:    "*net.TCPConn",
			InterfaceName: "net.Conn",
		},
	},
}

// InspectBinary reads the ELF/DWARF data from the given file,
// finding the needed attachment arguments and probe data
// needed to trace the Go standard library TLS functions.
func InspectBinary(elfFile *elf.File) (*attachmentArgs, error) {
	inspectionResult, err := bininspect.Inspect(elfFile, inspectConfig)
	if err != nil {
		return nil, fmt.Errorf("error while inspecting binary for TLS probe attachment data: %w", err)
	}

	attachmentArgs, err := convertToAttachmentArgs(inspectionResult)
	if err != nil {
		return nil, fmt.Errorf("error while converting inspection result to TLS probe attachment data: %w", err)
	}

	return attachmentArgs, nil
}

func convertToAttachmentArgs(inspectionData *bininspect.Result) (*attachmentArgs, error) {
	args := attachmentArgs{}

	args.loadGoroutineIDData(inspectionData.GoroutineIDMetadata)
	err := args.loadFunctions(inspectionData)
	if err != nil {
		return nil, err
	}

	// Add struct offsets
	if inspectionData.IncludesDebugSymbols {
		args.loadStructOffsets(inspectionData.StructOffsets)
	} else {
		err = args.loadFallbackStructOffsets(inspectionData.GoVersion, string(inspectionData.Arch))
		if err != nil {
			return nil, err
		}
	}

	// Add static itab entries
	if inspectionData.IncludesDebugSymbols {
		args.loadStaticItabEntries(inspectionData.StaticItabEntries)
	}

	return &args, nil
}

func (a *attachmentArgs) loadGoroutineIDData(goroutineIDData bininspect.GoroutineIDMetadata) {
	a.probeData.Goroutine_id = ebpf.GoroutineIDMetadata{
		Runtime_g_tls_addr_offset: goroutineIDData.RuntimeGTLSAddrOffset,
		Goroutine_id_offset:       goroutineIDData.GoroutineIDOffset,
		Runtime_g_register:        int64(goroutineIDData.RuntimeGRegister),
		Runtime_g_in_register:     B2i(goroutineIDData.RuntimeGInRegister),
	}
}

func (a *attachmentArgs) loadFunctions(inspectionData *bininspect.Result) error {
	lookingFor := map[string]struct{}{
		string(goTlsConnWrite): {},
		string(goTlsConnRead):  {},
		string(goTlsConnClose): {},
	}

	for _, f := range inspectionData.Functions {
		var err error
		switch f.Name {
		case string(goTlsConnWrite):
			err = a.loadConnWrite(f, inspectionData)
		case string(goTlsConnRead):
			err = a.loadConnRead(f, inspectionData)
		case string(goTlsConnClose):
			err = a.loadConnClose(f, inspectionData)
		default:
			return fmt.Errorf("an unknown function %q was found in inspection data", f.Name)
		}

		if err != nil {
			return fmt.Errorf("error while adding function metadata for %q: %w", f.Name, err)
		}

		delete(lookingFor, f.Name)
	}

	if len(lookingFor) > 0 {
		notFound := []string{}
		for k := range lookingFor {
			notFound = append(notFound, k)
		}
		return fmt.Errorf("required functions were not found in inspection data: %#v", notFound)
	}

	return nil
}

// func (c *Conn) Write(b []byte) (int, error)
func (a *attachmentArgs) loadConnWrite(function bininspect.FunctionMetadata, inspectionData *bininspect.Result) error {
	a.writeAddress = function.EntryLocation

	params := function.Parameters
	if !inspectionData.IncludesDebugSymbols {
		// Fall back to the lookup table values
		fallbackParams, err := lookup.GetWriteParams(inspectionData.GoVersion, string(inspectionData.Arch))
		if err != nil {
			return fmt.Errorf("error when using fallback lookup table for write params: %w", err)
		}
		params = fallbackParams
	}

	// Unpack the parameters
	if len(params) != 2 {
		return fmt.Errorf("number of parameters for %q was unexpected (%d)", function.Name, len(params))
	}

	// c is the pointer receiver (c crypto/tls.(*Conn))
	cParam := params[0]
	cLoc, err := wordLocation(cParam, inspectionData.Arch, "pointer", reflect.Ptr)
	if err != nil {
		return fmt.Errorf("error when finding location of c param in %q: %w", function.Name, err)
	}
	a.probeData.Write_conn_pointer = cLoc

	// b is the buffer (b []byte)
	bParam := params[1]
	bLoc, err := sliceLocation(bParam, inspectionData.Arch)
	if err != nil {
		return fmt.Errorf("error when finding location of b param in %q: %w", function.Name, err)
	}
	a.probeData.Write_buffer = bLoc

	return nil
}

// func (c *Conn) Read(b []byte) (int, error)
func (a *attachmentArgs) loadConnRead(function bininspect.FunctionMetadata, inspectionData *bininspect.Result) error {
	a.readAddress = function.EntryLocation

	if len(function.ReturnLocations) == 0 {
		return fmt.Errorf("no return locations found for %q", function.Name)
	}
	a.readReturnAddresses = make([]uint64, len(function.ReturnLocations))
	copy(a.readReturnAddresses, function.ReturnLocations)

	params := function.Parameters
	if !inspectionData.IncludesDebugSymbols {
		// Fall back to the lookup table values
		fallbackParams, err := lookup.GetReadParams(inspectionData.GoVersion, string(inspectionData.Arch))
		if err != nil {
			return fmt.Errorf("error when using fallback lookup table for read params: %w", err)
		}
		params = fallbackParams
	}

	// Unpack the parameters
	if len(params) != 2 {
		return fmt.Errorf("number of parameters for %q was unexpected (%d)", function.Name, len(params))
	}

	// c is the pointer receiver (c crypto/tls.(*Conn))
	cParam := params[0]
	cLoc, err := wordLocation(cParam, inspectionData.Arch, "pointer", reflect.Ptr)
	if err != nil {
		return fmt.Errorf("error when finding location of c param in %q: %w", function.Name, err)
	}
	a.probeData.Read_conn_pointer = cLoc

	// b is the buffer (b []byte)
	bParam := params[1]
	// Handle bug with Go 1.16.* where the location list for the Read.b parameter is empty
	// despite residing on the stack at the below offsets
	if inspectionData.GoVersion.Major == 1 && inspectionData.GoVersion.Minor == 16 && len(bParam.Pieces) == 0 {
		a.probeData.Read_buffer = ebpf.SliceLocation{
			Ptr: ebpf.Location{
				Exists:       B2i(true),
				In_register:  B2i(false),
				Stack_offset: 16,
			},
			Len: ebpf.Location{
				Exists:       B2i(true),
				In_register:  B2i(false),
				Stack_offset: 24,
			},
			Cap: ebpf.Location{
				Exists:       B2i(true),
				In_register:  B2i(false),
				Stack_offset: 32,
			},
		}
	} else {
		bLoc, err := sliceLocation(bParam, inspectionData.Arch)
		if err != nil {
			return fmt.Errorf("error when finding location of b param in %q: %w", function.Name, err)
		}
		a.probeData.Read_buffer = bLoc
	}

	// Manually re-consturct the location of the first return parameter (bytes read).
	// Unpack the first return parameter (bytes read).
	// The error return value isn't useful in eBPF
	// unless we can determine whether it is equal to io.EOF,
	// and I didn't find a straightforward way of doing this.
	//
	// Additionally, because the DWARF location lists return locations for the return values,
	// we're forced to manually determine their locations
	// by re-implementing the register allocation/stack layout algorithms
	// from the ABI specs.
	// As such, this region of code is especially sensitive to ABI changes.
	switch inspectionData.ABI {
	case bininspect.GoABIRegister:
		// Manually assign the registers.
		// This is fairly finnicky, but is simple
		// since the return arguments are short and are word-aligned
		var regOrder []int
		switch inspectionData.Arch {
		case bininspect.GoArchX86_64:
			// The order registers is assigned is in the below slice
			// (where each value is the register number):
			// From https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md
			// RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
			regOrder = []int{0, 3, 2, 5, 4, 8, 9, 10, 11}
		case bininspect.GoArchARM64:
			// TODO implement
			return fmt.Errorf("ARM-64 register ABI fallback not implemented")
		}

		curReg := 0
		getNextReg := func() int {
			nextReg := regOrder[curReg]
			curReg += 1
			return nextReg
		}

		a.probeData.Read_return_bytes = ebpf.Location{
			Exists:      B2i(true),
			In_register: B2i(true),
			X_register:  int64(getNextReg()),
		}
	case bininspect.GoABIStack:
		// Manually reconstruct the offsets into the stack.
		// Assume the return parameters exist on the stack in the stable struct,
		// adjacent to the parameters.
		// This is valid for go running ABI0/the stack ABI).
		// See:
		// - https://go.googlesource.com/proposal/+/refs/changes/78/248178/1/design/40724-register-calling.md#go_s-current-stack_based-abi
		// - https://dr-knz.net/go-calling-convention-x86-64-2020.html
		var endOfParametersOffset int64
		for _, param := range params {
			// This code assumes pointer alignment of each param
			endOfParametersOffset += param.TotalSize
		}

		currentOffset := endOfParametersOffset
		a.probeData.Read_return_bytes = ebpf.Location{
			Exists:       B2i(true),
			In_register:  B2i(false),
			Stack_offset: currentOffset,
		}
	}

	return nil
}

// func (c *Conn) Close() error
func (a *attachmentArgs) loadConnClose(function bininspect.FunctionMetadata, inspectionData *bininspect.Result) error {
	a.closeAddress = function.EntryLocation

	params := function.Parameters
	if !inspectionData.IncludesDebugSymbols {
		// Fall back to the lookup table values
		fallbackParams, err := lookup.GetCloseParams(inspectionData.GoVersion, string(inspectionData.Arch))
		if err != nil {
			return fmt.Errorf("error when using fallback lookup table for close params: %w", err)
		}
		params = fallbackParams
	}

	// Unpack the parameters
	if len(params) != 1 {
		return fmt.Errorf("number of parameters for %q was unexpected (%d)", function.Name, len(params))
	}

	// c is the pointer receiver (c crypto/tls.(*Conn))
	cParam := params[0]
	cLoc, err := wordLocation(cParam, inspectionData.Arch, "pointer", reflect.Ptr)
	if err != nil {
		return fmt.Errorf("error when finding location of c param in %q: %w", function.Name, err)
	}
	a.probeData.Close_conn_pointer = cLoc

	return nil
}

func (a *attachmentArgs) loadStructOffsets(structOffsets []bininspect.StructOffset) {
	for _, s := range structOffsets {
		if s.StructName == "crypto/tls.Conn" && s.FieldName == "conn" {
			a.probeData.Conn_layout.Tls_conn_inner_conn_offset = s.Offset
		} else if s.StructName == "net.TCPConn" && s.FieldName == "conn" {
			a.probeData.Conn_layout.Tcp_conn_inner_conn_offset = s.Offset
		} else if s.StructName == "net.conn" && s.FieldName == "fd" {
			a.probeData.Conn_layout.Conn_fd_offset = s.Offset
		} else if s.StructName == "net.netFD" && s.FieldName == "pfd" {
			a.probeData.Conn_layout.Net_fd_pfd_offset = s.Offset
		} else if s.StructName == "internal/poll.FD" && s.FieldName == "Sysfd" {
			a.probeData.Conn_layout.Fd_sysfd_offset = s.Offset
		}
	}
}

func (a *attachmentArgs) loadFallbackStructOffsets(goVersion goversion.GoVersion, arch string) error {
	fallbackOffset, err := lookup.GetTLSConnInnerConnOffset(goVersion, arch)
	if err != nil {
		return fmt.Errorf("error when using fallback lookup table for Tls_conn_inner_conn_offset: %w", err)
	}
	a.probeData.Conn_layout.Tls_conn_inner_conn_offset = fallbackOffset

	fallbackOffset, err = lookup.GetTCPConnInnerConnOffset(goVersion, arch)
	if err != nil {
		return fmt.Errorf("error when using fallback lookup table for Tcp_conn_inner_conn_offset: %w", err)
	}
	a.probeData.Conn_layout.Tcp_conn_inner_conn_offset = fallbackOffset

	fallbackOffset, err = lookup.GetConnFDOffset(goVersion, arch)
	if err != nil {
		return fmt.Errorf("error when using fallback lookup table for Conn_fd_offset: %w", err)
	}
	a.probeData.Conn_layout.Conn_fd_offset = fallbackOffset

	fallbackOffset, err = lookup.GetNetFD_PFDOffset(goVersion, arch)
	if err != nil {
		return fmt.Errorf("error when using fallback lookup table for Net_fd_pfd_offset: %w", err)
	}
	a.probeData.Conn_layout.Net_fd_pfd_offset = fallbackOffset

	fallbackOffset, err = lookup.GetFD_SysfdOffset(goVersion, arch)
	if err != nil {
		return fmt.Errorf("error when using fallback lookup table for Fd_sysfd_offset: %w", err)
	}
	a.probeData.Conn_layout.Fd_sysfd_offset = fallbackOffset

	return nil
}

func (a *attachmentArgs) loadStaticItabEntries(staticItabEntries []bininspect.StaticItabEntry) {
	for _, s := range staticItabEntries {
		if s.StructName == "*net.TCPConn" && s.InterfaceName == "net.Conn" {
			a.probeData.Conn_layout.Tcp_conn_interface_type = s.EntryIndex
		}
	}
}

func B2i(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func wordLocation(
	param bininspect.ParameterMetadata,
	arch bininspect.GoArch,
	typeName string,
	expectedKind reflect.Kind,
) (ebpf.Location, error) {
	if len(param.Pieces) == 0 {
		return ebpf.Location{Exists: B2i(false)}, nil
	}

	if len(param.Pieces) != 1 {
		return ebpf.Location{}, fmt.Errorf("expected 1 piece for %s parameter, got %d", typeName, len(param.Pieces))
	}
	if param.Kind != expectedKind {
		return ebpf.Location{}, fmt.Errorf("expected %#v kind for %s parameter, got %#v", expectedKind, typeName, param.Kind)
	}
	if param.TotalSize != int64(arch.PointerSize()) {
		return ebpf.Location{}, fmt.Errorf("expected total size for %s parameter to be %d, got %d", typeName, arch.PointerSize(), param.TotalSize)
	}

	piece := param.Pieces[0]
	return ebpf.Location{
		Exists:       B2i(true),
		In_register:  B2i(piece.InReg),
		Stack_offset: piece.StackOffset,
		X_register:   int64(piece.Register),
	}, nil
}

func compositeLocation(
	param bininspect.ParameterMetadata,
	arch bininspect.GoArch,
	typeName string,
	expectedKind reflect.Kind,
	expectedPieces int,
) ([]ebpf.Location, error) {
	if len(param.Pieces) == 0 {
		locations := make([]ebpf.Location, expectedPieces)
		for i := range locations {
			locations[i] = ebpf.Location{
				Exists: B2i(false),
			}
		}
		return locations, nil
	}

	if len(param.Pieces) < 1 || len(param.Pieces) > expectedPieces {
		return nil, fmt.Errorf("expected 1-%d pieces for %s parameter, got %d", expectedPieces, typeName, len(param.Pieces))
	}
	if param.Kind != expectedKind {
		return nil, fmt.Errorf("expected %#v kind for %s parameter, got %#v", expectedKind, typeName, param.Kind)
	}
	expectedSize := int64(int(arch.PointerSize()) * expectedPieces)
	if param.TotalSize != expectedSize {
		return nil, fmt.Errorf("expected total size for %s parameter to be %d, got %d", typeName, expectedSize, param.TotalSize)
	}

	// Translate the parameter pieces to a list of single word locations
	// TODO handle missing inner parts
	//      like the length (seems to handle missing cap)
	locations := make([]ebpf.Location, expectedPieces)
	currentLocation := 0
	for i, paramPiece := range param.Pieces {
		if paramPiece.InReg {
			if paramPiece.Size > int64(arch.PointerSize()) {
				return nil, fmt.Errorf("piece %d in %s parameter was in register but longer than %d bytes", i, typeName, arch.PointerSize())
			}

			locations[currentLocation] = ebpf.Location{
				Exists:      B2i(true),
				In_register: B2i(true),
				X_register:  int64(paramPiece.Register),
			}
			currentLocation += 1
		} else {
			// If the parameter piece is longer than a word,
			// divide it into multiple single-word locations
			var currentOffset int64
			remainingLength := paramPiece.Size
			for remainingLength > 0 {
				locations[currentLocation] = ebpf.Location{
					Exists:       B2i(true),
					In_register:  B2i(false),
					Stack_offset: paramPiece.StackOffset + currentOffset,
				}
				currentLocation += 1
				currentOffset += int64(arch.PointerSize())
				if remainingLength >= int64(arch.PointerSize()) {
					remainingLength -= int64(arch.PointerSize())
				} else {
					remainingLength = 0
				}
			}
		}
	}

	// Handle any trailing locations that don't exist
	if currentLocation != expectedPieces-1 {
		for ; currentLocation < expectedPieces; currentLocation++ {
			locations[expectedPieces] = ebpf.Location{
				Exists: B2i(false),
			}
		}
	}

	return locations, nil
}

func sliceLocation(param bininspect.ParameterMetadata, arch bininspect.GoArch) (ebpf.SliceLocation, error) {
	locations, err := compositeLocation(param, arch, "slice", reflect.Slice, 3)
	if err != nil {
		return ebpf.SliceLocation{}, err
	}

	return ebpf.SliceLocation{
		Ptr: locations[0],
		Len: locations[1],
		Cap: locations[2],
	}, nil
}
