package diconfig

import (
	"debug/elf"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
)

// inspectGoBinaries goes through each service and populates information about the binary
// and the relevant parameters, and their types
// configEvent maps service names to info about the service and their configurations
func inspectGoBinaries(configEvent ditypes.DIProcs) error {
	var err error
	for i := range configEvent {
		err = analyzeBinary(configEvent[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func analyzeBinary(procInfo *ditypes.ProcessInfo) error {
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
		fieldIDs = append(fieldIDs, collectFieldIDs(funcParams)...)
	}

	r, err := bininspect.InspectWithDWARF(elfFile, functions, fieldIDs)
	if err != nil {
		return fmt.Errorf("could not determine locations of variables from debug information %w", err)
	}

	// Use the result from InspectWithDWARF to populate the locations of parameters
	for functionName, functionMetadata := range r.Functions {
		putLocationsInParams(functionMetadata.Parameters, procInfo.TypeMap.Functions, functionName)
		correctStructSizes(procInfo.TypeMap.Functions[functionName])
	}

	for _, v := range typeMap.Functions {
		addPointerOffsets(r.StructOffsets, v)
	}

	return nil
}

func addPointerOffsets(offsets map[bininspect.FieldIdentifier]uint64, params []ditypes.Parameter) {

	stack := []*ditypes.Parameter{}
	for i := range params {
		stack = append(stack, &params[i])
	}

	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for n := range current.ParameterPieces {
			stack = append(stack, &current.ParameterPieces[n])
		}

		if current.Kind == uint(reflect.Struct) {
			for i := range current.ParameterPieces {

				fieldID := bininspect.FieldIdentifier{
					StructName: current.Type,
					FieldName:  current.ParameterPieces[i].Name,
				}
				x, ok := offsets[fieldID]
				if !ok {
					continue
				}
				current.ParameterPieces[i].Location.PointerOffset = x
			}
		}
	}
}

// collectFieldIDs returns all struct fields if there are any amongst types of parameters
// including if there's structs that are nested deep within complex types
func collectFieldIDs(params []ditypes.Parameter) []bininspect.FieldIdentifier {

	fieldIDs := []bininspect.FieldIdentifier{}
	stack := append([]ditypes.Parameter{}, params...)

	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if len(current.ParameterPieces) != 0 {
			stack = append(stack, current.ParameterPieces...)
		}

		if current.Kind == uint(reflect.Struct) || current.Kind == uint(reflect.Slice) {
			for _, structField := range current.ParameterPieces {
				if structField.Name == "" {
					continue
				}
				fieldIDs = append(fieldIDs, bininspect.FieldIdentifier{
					StructName: current.Type,
					FieldName:  structField.Name,
				})
			}
		}

	}
	return fieldIDs
}

func putLocationsInParams(paramMetadatas []bininspect.ParameterMetadata, funcMap map[string][]ditypes.Parameter, funcName string) {
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
	correctArrayLocations(params)
	correctPointerLocations(params)

	funcMap[funcName] = params
}

func assignLocationsInOrder(params []ditypes.Parameter, locations []ditypes.Location) {
	queue := []*ditypes.Parameter{}
	locationCounter := 0

	// Start by pushing addresses of all parameters to queue
	for i := range params {
		queue = append(queue, &params[i])
	}

	for {
		if len(queue) == 0 || locationCounter == len(locations) {
			return
		}

		current := queue[0]
		queue = queue[1:]
		if len(current.ParameterPieces) != 0 && current.Kind != uint(reflect.Array) && current.Kind != uint(reflect.Pointer) {
			for i := range current.ParameterPieces {
				queue = append(queue, &current.ParameterPieces[i])
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
			}
			locationCounter++
		}
	}
}

// correctArrayLocations finds all array types throughout the parameter tree and assigns
// the array's offset to individual elements.
//
// There's only one location assigned for arrays by DWARF, as opposed to
// one for each individual element. We have to take the starting offset
// for the array and distribute it down through individual, possibly deeply
// embedded elements
func correctArrayLocations(params []ditypes.Parameter) {
	queue := []*ditypes.Parameter{}

	for i := range params {
		queue = append(queue, &params[i])
	}

	var current *ditypes.Parameter
	for len(queue) != 0 {

		current = queue[0]
		queue = queue[1:]

		if len(current.ParameterPieces) == 0 {
			continue
		}

		if current.Kind == uint(reflect.Array) {
			// for array elements, update their location based on the array stack offset
			startingOffset := current.Location.StackOffset
			pieceSize := current.ParameterPieces[0].TotalSize

			for n := range current.ParameterPieces {
				// correct name for array elements, otherwise it's [len]type
				current.ParameterPieces[n].Name = fmt.Sprintf("%s_%d", current.Name, n)
				current.ParameterPieces[n].Location = ditypes.Location{
					StackOffset: (pieceSize * int64(n)) + startingOffset,
				}
			}
		}

		// enqueue all children
		for j := range current.ParameterPieces {
			queue = append(queue, &current.ParameterPieces[j])
		}
	}
}

func correctPointerLocations(params []ditypes.Parameter) {

	stack := []*ditypes.Parameter{}

	// Start by pushing addresses of all parameters to stack
	for i := range params {
		stack = append(stack, &params[i])
	}

	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if len(current.ParameterPieces) != 0 {
			for i := range current.ParameterPieces {
				stack = append(stack, &current.ParameterPieces[i])
			}
		}

		if current.Kind == uint(reflect.Pointer) {
			// Distribute the pointer location to ALL subfields of this pointer,
			// not just the first layer
			pointerQueue := []*ditypes.Parameter{}
			for n := range current.ParameterPieces {
				pointerQueue = append(pointerQueue, &current.ParameterPieces[n])
			}

			for len(pointerQueue) != 0 {
				currentPointer := pointerQueue[0]
				pointerQueue = pointerQueue[1:]

				currentPointer.Location = current.Location
				currentPointer.Location.NeedsDereference = true
				for m := range currentPointer.ParameterPieces {
					pointerQueue = append(pointerQueue, &currentPointer.ParameterPieces[m])
				}
			}

			current.Location.NeedsDereference = false
		}
	}
}
