// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls"
) // TODO: Convert binID to goTLSBinaryID

func inspectionResultToProbeData(result *bininspect.Result) (gotls.TlsOffsetsData, error) {
	closeConnPointer, err := getConnPointer(result, bininspect.CloseGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting close conn pointer from inspection result: %w", err)
	}
	readConnPointer, err := getConnPointer(result, bininspect.ReadGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting read conn pointer from inspection result: %w", err)
	}
	readBufferLocation, err := getReadBufferLocation(result)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting read buffer location from inspection result: %w", err)
	}
	readReturnBytes, err := getReturnBytes(result, bininspect.ReadGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting read return bytes from inspection result: %w", err)
	}
	writeConnPointer, err := getConnPointer(result, bininspect.WriteGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting write conn pointer from inspection result: %w", err)
	}
	writeBufferLocation, err := getWriteBufferLocation(result)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting write buffer location from inspection result: %w", err)
	}
	writeReturnBytes, err := getReturnBytes(result, bininspect.WriteGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting read return bytes from inspection result: %w", err)
	}
	writeReturnError, err := getReturnError(result, bininspect.WriteGoTLSFunc)
	if err != nil {
		return gotls.TlsOffsetsData{}, fmt.Errorf("failed extracting read return error from inspection result: %w", err)
	}

	return gotls.TlsOffsetsData{
		Goroutine_id: gotls.GoroutineIDMetadata{
			Runtime_g_tls_addr_offset: result.GoroutineIDMetadata.RuntimeGTLSAddrOffset,
			Goroutine_id_offset:       result.GoroutineIDMetadata.GoroutineIDOffset,
			Runtime_g_register:        int64(result.GoroutineIDMetadata.RuntimeGRegister),
			Runtime_g_in_register:     boolToBinary(result.GoroutineIDMetadata.RuntimeGInRegister),
		},
		Conn_layout: gotls.TlsConnLayout{
			Tls_conn_inner_conn_offset: result.StructOffsets[bininspect.StructOffsetTLSConn],
			Tcp_conn_inner_conn_offset: result.StructOffsets[bininspect.StructOffsetTCPConn],
			Conn_fd_offset:             result.StructOffsets[bininspect.StructOffsetNetConnFd],
			Net_fd_pfd_offset:          result.StructOffsets[bininspect.StructOffsetNetFdPfd],
			Fd_sysfd_offset:            result.StructOffsets[bininspect.StructOffsetPollFdSysfd],
		},
		Read_conn_pointer:  readConnPointer,
		Read_buffer:        readBufferLocation,
		Read_return_bytes:  readReturnBytes,
		Write_conn_pointer: writeConnPointer,
		Write_buffer:       writeBufferLocation,
		Write_return_bytes: writeReturnBytes,
		Write_return_error: writeReturnError,
		Close_conn_pointer: closeConnPointer,
	}, nil
}

func getConnPointer(result *bininspect.Result, funcName string) (gotls.Location, error) {
	if len(result.Functions[funcName].Parameters) < 1 {
		return gotls.Location{}, errors.New("expected at least one parameter")
	}
	readConnReceiver := result.Functions[funcName].Parameters[0]
	return wordLocation(readConnReceiver, result.Arch, "pointer", reflect.Ptr)
}

func getReadBufferLocation(result *bininspect.Result) (gotls.SliceLocation, error) {
	params := result.Functions[bininspect.ReadGoTLSFunc].Parameters
	if len(params) < 2 {
		return gotls.SliceLocation{}, errors.New("expected at least two parameters for read function")
	}
	bufferParam := params[1]
	if result.GoVersion.Major == 1 && result.GoVersion.Minor == 16 && len(bufferParam.Pieces) == 0 {
		return gotls.SliceLocation{
			Ptr: gotls.Location{
				Exists:       boolToBinary(true),
				In_register:  boolToBinary(false),
				Stack_offset: 16,
			},
			Len: gotls.Location{
				Exists:       boolToBinary(true),
				In_register:  boolToBinary(false),
				Stack_offset: 24,
			},
			Cap: gotls.Location{
				Exists:       boolToBinary(true),
				In_register:  boolToBinary(false),
				Stack_offset: 32,
			},
		}, nil
	}
	return sliceLocation(bufferParam, result.Arch)
}

func getWriteBufferLocation(result *bininspect.Result) (gotls.SliceLocation, error) {
	params := result.Functions[bininspect.WriteGoTLSFunc].Parameters
	if len(params) < 2 {
		return gotls.SliceLocation{}, errors.New("expected at least two parameters in write function")
	}
	bufferParam := params[1]
	return sliceLocation(bufferParam, result.Arch)
}

func getReturnBytes(result *bininspect.Result, funcName string) (gotls.Location, error) {
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
	switch result.ABI {
	case bininspect.GoABIRegister:
		// Manually assign the registers.
		// This is fairly finnicky, but is simple
		// since the return arguments are short and are word-aligned
		switch result.Arch {
		case bininspect.GoArchX86_64:
			// The order registers is assigned is in the below slice
			// (where each value is the register number):
			// From https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md
			// RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
			// regOrder = []int{0, 3, 2, 5, 4, 8, 9, 10, 11}
			return gotls.Location{
				Exists:      boolToBinary(true),
				In_register: boolToBinary(true),
				X_register:  int64(0), // RAX
			}, nil
		case bininspect.GoArchARM64:
			return gotls.Location{
				Exists:      boolToBinary(true),
				In_register: boolToBinary(true),
				X_register:  int64(0),
			}, nil
		default:
			return gotls.Location{}, bininspect.ErrUnsupportedArch
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
		for _, param := range result.Functions[funcName].Parameters {
			// This code assumes pointer alignment of each param
			endOfParametersOffset += param.TotalSize
		}

		return gotls.Location{
			Exists:       boolToBinary(true),
			In_register:  boolToBinary(false),
			Stack_offset: endOfParametersOffset,
		}, nil
	default:
		return gotls.Location{}, fmt.Errorf("unknown abi %q", result.ABI)
	}
}

func getReturnError(result *bininspect.Result, funcName string) (gotls.Location, error) {
	switch result.ABI {
	case bininspect.GoABIRegister:
		switch result.Arch {
		case bininspect.GoArchX86_64:
			return gotls.Location{
				Exists:      boolToBinary(true),
				In_register: boolToBinary(true),
				X_register:  int64(3), // RBX
			}, nil
		case bininspect.GoArchARM64:
			return gotls.Location{
				Exists:      boolToBinary(true),
				In_register: boolToBinary(true),
				X_register:  int64(1),
			}, nil
		default:
			return gotls.Location{}, bininspect.ErrUnsupportedArch
		}
	case bininspect.GoABIStack:
		var integer int
		var endOfParametersOffset int64
		for _, param := range result.Functions[funcName].Parameters {
			// This code assumes pointer alignment of each param
			endOfParametersOffset += param.TotalSize
		}
		return gotls.Location{
			Exists:      boolToBinary(true),
			In_register: boolToBinary(false),
			// Take the offset of the first return value (an int representing the amount of bytes that were
			// read / written) and add the size of int to get the beginning of the next parameter (the error).
			Stack_offset: endOfParametersOffset + int64(unsafe.Sizeof(integer)),
		}, nil
	default:
		return gotls.Location{}, fmt.Errorf("unknown abi %q", result.ABI)
	}
}

func makeReturnUID(uid string, returnNumber int) string {
	return fmt.Sprintf("%s_%x", uid, returnNumber)
}

func boolToBinary(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func wordLocation(
	param bininspect.ParameterMetadata,
	arch bininspect.GoArch,
	typeName string,
	expectedKind reflect.Kind,
) (gotls.Location, error) {
	if len(param.Pieces) == 0 {
		return gotls.Location{Exists: boolToBinary(false)}, nil
	}

	if len(param.Pieces) != 1 {
		return gotls.Location{}, fmt.Errorf("expected 1 piece for %s parameter, got %d", typeName, len(param.Pieces))
	}
	if param.Kind != expectedKind {
		return gotls.Location{}, fmt.Errorf("expected %#v kind for %s parameter, got %#v", expectedKind, typeName, param.Kind)
	}
	if param.TotalSize != int64(arch.PointerSize()) {
		return gotls.Location{}, fmt.Errorf("expected total size for %s parameter to be %d, got %d", typeName, arch.PointerSize(), param.TotalSize)
	}

	piece := param.Pieces[0]
	return gotls.Location{
		Exists:       boolToBinary(true),
		In_register:  boolToBinary(piece.InReg),
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
) ([]gotls.Location, error) {
	if len(param.Pieces) == 0 {
		locations := make([]gotls.Location, expectedPieces)
		for i := range locations {
			locations[i] = gotls.Location{
				Exists: boolToBinary(false),
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
	locations := make([]gotls.Location, expectedPieces)
	currentLocation := 0
	for i, paramPiece := range param.Pieces {
		if paramPiece.InReg {
			if paramPiece.Size > int64(arch.PointerSize()) {
				return nil, fmt.Errorf("piece %d in %s parameter was in register but longer than %d bytes", i, typeName, arch.PointerSize())
			}

			locations[currentLocation] = gotls.Location{
				Exists:      boolToBinary(true),
				In_register: boolToBinary(true),
				X_register:  int64(paramPiece.Register),
			}
			currentLocation += 1
		} else {
			// If the parameter piece is longer than a word,
			// divide it into multiple single-word locations
			var currentOffset int64
			remainingLength := paramPiece.Size
			for remainingLength > 0 {
				locations[currentLocation] = gotls.Location{
					Exists:       boolToBinary(true),
					In_register:  boolToBinary(false),
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
			locations[expectedPieces] = gotls.Location{
				Exists: boolToBinary(false),
			}
		}
	}

	return locations, nil
}

func sliceLocation(param bininspect.ParameterMetadata, arch bininspect.GoArch) (gotls.SliceLocation, error) {
	// We expect each slice golang parameter to have 3 parts - the ptr to the data, the length and the capacity.
	locations, err := compositeLocation(param, arch, "slice", reflect.Slice, 3)
	if err != nil {
		return gotls.SliceLocation{}, err
	}

	return gotls.SliceLocation{
		Ptr: locations[0],
		Len: locations[1],
		Cap: locations[2],
	}, nil
}
