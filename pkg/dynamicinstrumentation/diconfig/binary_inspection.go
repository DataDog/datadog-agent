// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"debug/elf"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
)

// inspectGoBinaries goes through each service and populates information about the binary
// and the relevant parameters, and their types
// configEvent maps service names to info about the service and their configurations
func inspectGoBinaries(configEvent ditypes.DIProcs) error {
	var err error
	for i := range configEvent {
		err = AnalyzeBinary(configEvent[i])
		if err != nil {
			return fmt.Errorf("inspection of PID %d (path=%s) failed: %w", configEvent[i].PID, configEvent[i].BinaryPath, err)
		}
	}
	return nil
}

// AnalyzeBinary reads the binary associated with the specified process and parses
// the DWARF information. It populates relevant fields in the process representation
func AnalyzeBinary(procInfo *ditypes.ProcessInfo) error {
	functions := []string{}
	targetFunctions := map[string]bool{}
	for _, probe := range procInfo.GetProbes() {
		functions = append(functions, probe.FuncName)
		targetFunctions[probe.FuncName] = true
	}

	dwarfData, err := loadDWARF(procInfo.BinaryPath)
	if err != nil {
		return fmt.Errorf("could not retrieve debug information from binary: %w", err)
	}

	typeMap, err := getTypeMap(dwarfData, targetFunctions)
	if err != nil {
		return fmt.Errorf("could not retrieve type information from binary %w", err)
	}

	procInfo.TypeMap = typeMap

	elfFile, err := elf.Open(procInfo.BinaryPath)
	if err != nil {
		return fmt.Errorf("could not open elf file %w", err)
	}

	procInfo.DwarfData = dwarfData

	fieldIDs := make([]bininspect.FieldIdentifier, 0)
	for _, funcParams := range typeMap.Functions {
		for _, param := range funcParams {
			fieldIDs = append(fieldIDs,
				collectFieldIDs(param)...)
		}
	}

	r, err := bininspect.InspectWithDWARF(elfFile, functions, fieldIDs)
	if err != nil {
		return fmt.Errorf("could not determine locations of variables from debug information %w", err)
	}

	// Use the result from InspectWithDWARF to populate the locations of parameters
	for functionName, functionMetadata := range r.Functions {
		putLocationsInParams(functionMetadata.Parameters, r.StructOffsets, procInfo.TypeMap.Functions, functionName)
		correctStructSizes(procInfo.TypeMap.Functions[functionName])
	}

	return nil
}

// collectFieldIDs returns all struct fields if there are any amongst types of parameters
// including if there's structs that are nested deep within complex types
func collectFieldIDs(param ditypes.Parameter) []bininspect.FieldIdentifier {
	fieldIDs := []bininspect.FieldIdentifier{}
	stack := append([]ditypes.Parameter{param}, param.ParameterPieces...)

	for len(stack) != 0 {

		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !kindIsSupported(reflect.Kind(current.Kind)) {
			continue
		}
		if len(current.ParameterPieces) != 0 {
			stack = append(stack, current.ParameterPieces...)
		}

		if current.Kind == uint(reflect.Struct) || current.Kind == uint(reflect.Slice) {
			for _, structField := range current.ParameterPieces {
				if structField.Name == "" || current.Type == "" {
					// these can be blank in anonymous types or embedded fields
					// of builtin types. bininspect has no ability to find offsets
					// in these cases and we're best off skipping them.
					continue
				}
				fieldIDs = append(fieldIDs, bininspect.FieldIdentifier{
					StructName: current.Type,
					FieldName:  structField.Name,
				})
				if len(fieldIDs) >= ditypes.MaxFieldCount {
					log.Info("field limit applied, not collecting further fields", len(fieldIDs), ditypes.MaxFieldCount)
					return fieldIDs
				}
			}
		}
	}
	return fieldIDs
}

func putLocationsInParams(
	paramMetadatas []bininspect.ParameterMetadata,
	fieldLocations map[bininspect.FieldIdentifier]uint64,
	funcMap map[string][]ditypes.Parameter,
	funcName string) {

	params := funcMap[funcName]
	locations := []ditypes.Location{}

	// Collect locations in order
	for _, param := range paramMetadatas {
		for _, piece := range param.Pieces {
			locations = append(locations, ditypes.Location{
				InReg:       piece.InReg,
				StackOffset: piece.StackOffset,
				Register:    piece.Register,
			})
		}
	}

	assignLocationsInOrder(params, locations)
	correctTypeSpecificLocations(params, fieldLocations)

	funcMap[funcName] = params
}

func assignLocationsInOrder(params []ditypes.Parameter, locations []ditypes.Location) {
	stack := []*ditypes.Parameter{}
	locationCounter := 0

	// Start by pushing addresses of all parameters to stack
	for i := range params {
		stack = append(stack, &params[len(params)-1-i])
	}

	for {
		if len(stack) == 0 || locationCounter == len(locations) {
			return
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(current.ParameterPieces) != 0 &&
			current.Kind != uint(reflect.Array) &&
			current.Kind != uint(reflect.Pointer) &&
			current.Kind != uint(reflect.Slice) {

			for i := range current.ParameterPieces {
				stack = append(stack, &current.ParameterPieces[len(current.ParameterPieces)-1-i])
			}
		} else {
			// Location fields are directly assigned instead of setting the whole
			// location field to preserve other fields
			locationToAssign := locations[locationCounter]
			current.Location.InReg = locationToAssign.InReg
			current.Location.Register = locationToAssign.Register
			current.Location.StackOffset = locationToAssign.StackOffset

			if reflect.Kind(current.Kind) == reflect.String {
				// Strings actually have two locations (pointer, length)
				// but are shortened to a single one for parsing. The missing
				// location is taken into account in bpf code, but we need
				// to make sure it's not assigned to something else here.
				locationCounter++
			} else if reflect.Kind(current.Kind) == reflect.Slice {
				// slices actually have three locations (array, length, capacity)
				// but are shortened to a single one for parsing. The missing
				// locations are taken into account in bpf code, but we need
				// to make sure it's not assigned to something else here.
				locationCounter += 2
			}
			locationCounter++
		}
	}
}

func correctTypeSpecificLocations(params []ditypes.Parameter, fieldLocations map[bininspect.FieldIdentifier]uint64) {
	for i := range params {
		if params[i].Kind == uint(reflect.Array) {
			correctArrayLocations(&params[i], fieldLocations)
		} else if params[i].Kind == uint(reflect.Pointer) {
			correctPointerLocations(&params[i], fieldLocations)
		} else if params[i].Kind == uint(reflect.Struct) {
			correctStructLocations(&params[i], fieldLocations)
		}
	}
}

// correctStructLocations sets pointer and stack offsets for struct fields from
// bininspect results
func correctStructLocations(structParam *ditypes.Parameter, fieldLocations map[bininspect.FieldIdentifier]uint64) {
	for i := range structParam.ParameterPieces {
		fieldID := bininspect.FieldIdentifier{
			StructName: structParam.Type,
			FieldName:  structParam.ParameterPieces[i].Name,
		}
		offset, ok := fieldLocations[fieldID]
		if !ok {
			log.Infof("no field location available for %s.%s\n", fieldID.StructName, fieldID.FieldName)
			continue
		}

		fieldLocationsHaveAlreadyBeenDirectlyAssigned := isLocationSet(structParam.ParameterPieces[i].Location)
		if fieldLocationsHaveAlreadyBeenDirectlyAssigned {
			// The location would be set if it was directly assigned to (i.e. has its own register instead of needing
			// to dereference a pointer or get the element from a slice)
			structParam.ParameterPieces[i].Location = structParam.Location
			structParam.ParameterPieces[i].Location.StackOffset = int64(offset) + structParam.Location.StackOffset
		}

		structParam.ParameterPieces[i].Location.PointerOffset = offset
		structParam.ParameterPieces[i].Location.StackOffset = structParam.ParameterPieces[0].Location.StackOffset + int64(offset)

		correctTypeSpecificLocations([]ditypes.Parameter{structParam.ParameterPieces[i]}, fieldLocations)
	}
}

func isLocationSet(l ditypes.Location) bool {
	return reflect.DeepEqual(l, ditypes.Location{})
}

// correctPointerLocations takes a parameters location and copies it to the underlying
// type that's pointed to. It sets `NeedsDereference` to true
// then calls the top level function on each element of the array to ensure all
// element's have corrected locations
func correctPointerLocations(pointerParam *ditypes.Parameter, fieldLocations map[bininspect.FieldIdentifier]uint64) {
	// Pointers should have exactly one entry in ParameterPieces that correspond to the underlying type
	if len(pointerParam.ParameterPieces) != 1 {
		return
	}
	pointerParam.ParameterPieces[0].Location = pointerParam.Location
	pointerParam.ParameterPieces[0].Location.NeedsDereference = true
	correctTypeSpecificLocations([]ditypes.Parameter{pointerParam.ParameterPieces[0]}, fieldLocations)
}

// correctArrayLocations takes a parameter's location, and distribute it to each element
// by using `stack offset + (size*index)` then calls the top level function on each element
// of the array to ensure all element's have corrected locations
func correctArrayLocations(arrayParam *ditypes.Parameter, fieldLocations map[bininspect.FieldIdentifier]uint64) {
	initialOffset := arrayParam.Location.StackOffset
	for i := range arrayParam.ParameterPieces {
		arrayParam.ParameterPieces[i].Location.StackOffset = initialOffset + (arrayParam.ParameterPieces[i].TotalSize * int64(i))
		correctTypeSpecificLocations([]ditypes.Parameter{arrayParam.ParameterPieces[i]}, fieldLocations)
	}
}
