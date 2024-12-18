// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
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

	elfFile, err := safeelf.Open(procInfo.BinaryPath)
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
	stringPtrIdentifier := bininspect.FieldIdentifier{StructName: "string", FieldName: "str"}
	stringLenIdentifier := bininspect.FieldIdentifier{StructName: "string", FieldName: "len"}
	r.StructOffsets[stringPtrIdentifier] = 0
	r.StructOffsets[stringLenIdentifier] = 8

	// Use the result from InspectWithDWARF to populate the locations of parameters
	for functionName, functionMetadata := range r.Functions {
		putLocationsInParams(functionMetadata.Parameters, r.StructOffsets, procInfo.TypeMap.Functions, functionName)
		populateLocationExpressions(r.Functions, procInfo)
		correctStructSizes(procInfo.TypeMap.Functions[functionName])
	}

	return nil
}

// collectFieldIDs returns all struct fields if there are any amongst types of parameters
// including if there's structs that are nested deep within complex types
func collectFieldIDs(param *ditypes.Parameter) []bininspect.FieldIdentifier {
	fieldIDs := []bininspect.FieldIdentifier{}
	stack := append([]*ditypes.Parameter{param}, param.ParameterPieces...)

	for len(stack) != 0 {

		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || !kindIsSupported(reflect.Kind(current.Kind)) {
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

func populateLocationExpressions(
	metadata map[string]bininspect.FunctionMetadata,
	procInfo *ditypes.ProcessInfo) error {

	functions := procInfo.TypeMap.Functions
	probes := procInfo.GetProbes()
	funcNamesToLimits := map[string]*ditypes.InstrumentationInfo{}
	for i := range probes {
		funcNamesToLimits[probes[i].FuncName] = probes[i].InstrumentationInfo
	}

	for funcName, parameters := range functions {
		funcMetadata, ok := metadata[funcName]
		if !ok {
			return fmt.Errorf("no function metadata for function %s", funcName)
		}
		limitInfo, ok := funcNamesToLimits[funcName]
		if !ok || limitInfo == nil {
			continue
		}
		for i := range parameters {
			if i >= len(funcMetadata.Parameters) {
				return errors.New("parameter metadata does not line up with parameter itself")
			}
			GenerateLocationExpression(limitInfo, parameters[i])
		}
	}
	return nil
}

func putLocationsInParams(
	paramMetadatas []bininspect.ParameterMetadata,
	fieldLocations map[bininspect.FieldIdentifier]uint64,
	funcMap map[string][]*ditypes.Parameter,
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
	for i := range params {
		correctStructLocations(params[i], fieldLocations)
	}
	funcMap[funcName] = params
}

func assignLocationsInOrder(params []*ditypes.Parameter, locations []ditypes.Location) {
	stack := []*ditypes.Parameter{}
	locationCounter := 0

	// Start by pushing addresses of all parameters to stack
	for i := range params {
		stack = append(stack, params[len(params)-1-i])
	}

	for {
		if len(stack) == 0 || locationCounter >= len(locations) {
			return
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if len(current.ParameterPieces) != 0 &&
			current.Kind != uint(reflect.Array) &&
			current.Kind != uint(reflect.Pointer) {
			for i := range current.ParameterPieces {
				stack = append(stack, current.ParameterPieces[len(current.ParameterPieces)-1-i])
			}
		} else {
			// Location fields are directly assigned instead of setting the whole
			// location field to preserve other fields
			locationToAssign := locations[locationCounter]
			if current.Location == nil {
				current.Location = &ditypes.Location{}
			}
			current.Location.InReg = locationToAssign.InReg
			current.Location.Register = locationToAssign.Register
			current.Location.StackOffset = locationToAssign.StackOffset
			locationCounter++
		}
	}
}

// correctStructLocations field offsets for structs
func correctStructLocations(structParam *ditypes.Parameter, fieldLocations map[bininspect.FieldIdentifier]uint64) {
	for i := range structParam.ParameterPieces {
		if structParam.ParameterPieces[i] == nil {
			continue
		}

		fieldID := bininspect.FieldIdentifier{
			StructName: structParam.Type,
			FieldName:  structParam.ParameterPieces[i].Name,
		}
		offset := fieldLocations[fieldID]
		structParam.ParameterPieces[i].FieldOffset = offset
		correctStructLocations(structParam.ParameterPieces[i], fieldLocations)
	}
}

func isLocationSet(l ditypes.Location) bool {
	return reflect.DeepEqual(l, ditypes.Location{})
}
