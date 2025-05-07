// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"fmt"
	"reflect"
	"strings"

	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GenerateLocationExpression takes metadata about a parameter, including its type and location, and generates a list of
// LocationExpressions that can be used to read the parameter from the target process.
//
// It walks the tree of the parameter and its pieces, generating LocationExpressions for each piece.
//
//nolint:revive
func GenerateLocationExpression(limitsInfo *ditypes.InstrumentationInfo, param *ditypes.Parameter) {
	triePaths, expressionTargets := generateLocationVisitsMap(param)
	getParamFromTriePaths := func(pathElement string) *ditypes.Parameter {
		for n := range triePaths {
			if triePaths[n].TypePath == pathElement {
				return triePaths[n].Parameter
			}
		}
		return nil
	}
	seenPointers := map[string]bool{}
	// Go through each target type/field which needs to be captured
	for i := range expressionTargets {
		pathToInstrumentationTarget, instrumentationTarget := expressionTargets[i].TypePath, expressionTargets[i].Parameter

		targetExpressions := []ditypes.LocationExpression{}
		pathElements := []string{pathToInstrumentationTarget}
		// pathElements gets populated with every individual stretch of the path to the instrumentation target
		for {
			lastElementIndex := strings.LastIndex(pathToInstrumentationTarget, "@")
			if lastElementIndex == -1 {
				break
			}
			pathToInstrumentationTarget = pathToInstrumentationTarget[:lastElementIndex]
			pathElements = append([]string{pathToInstrumentationTarget}, pathElements...)
		}

		// Go through each path element of the instrumentation target
		for pathElementIndex := range pathElements {
			var elementParam *ditypes.Parameter = getParamFromTriePaths(pathElements[pathElementIndex])
			if elementParam == nil {
				log.Infof("Path not found to target: %s", pathElements[pathElementIndex])
				continue
			}
			// Check if this instrumentation target is directly assigned
			if elementParam.Location != nil {
				// Type is directly assigned
				if elementParam.Kind == uint(reflect.Array) {
					if elementParam.TotalSize == 0 && len(elementParam.ParameterPieces) == 0 {
						continue
					}
					GenerateLocationExpression(limitsInfo, elementParam.ParameterPieces[0])
					expressionsToUseForEachArrayElement := collectAllLocationExpressions(elementParam.ParameterPieces[0], true)
					targetExpressions = append(targetExpressions,
						// Read process stack address to the stack
						ditypes.ReadRegisterLocationExpression(ditypes.StackRegister, 8),
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.Location.StackOffset)),
					)
					//FIXME: Do we need to limit lengths of arrays??
					for i := 0; i < len(elementParam.ParameterPieces); i++ {
						targetExpressions = append(targetExpressions,
							ditypes.CopyLocationExpression(),
							ditypes.ApplyOffsetLocationExpression(uint(i*(int(elementParam.ParameterPieces[0].TotalSize)))),
						)
						targetExpressions = append(targetExpressions, expressionsToUseForEachArrayElement...)
					}
				} else if elementParam.Kind == uint(reflect.Pointer) {
					targetExpressions = append(targetExpressions,
						ditypes.DirectReadLocationExpression(elementParam),
					)
					_, ok := seenPointers[elementParam.ID]
					if !ok {
						targetExpressions = append(targetExpressions, ditypes.PopPointerAddressCompoundLocationExpression())
						seenPointers[elementParam.ID] = true
					}
				} else if elementParam.Kind == uint(reflect.Struct) {
					// Structs can have directly assigned locations if passed on the stack (common in the case of large structs)
					targetExpressions = append(targetExpressions,
						ditypes.ReadRegisterLocationExpression(ditypes.StackRegister, 8),
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.Location.StackOffset)),
					)
				} else {
					targetExpressions = append(targetExpressions,
						ditypes.DirectReadLocationExpression(elementParam),
						ditypes.PopLocationExpression(1, uint(elementParam.TotalSize)),
					)
				}
				continue
			} else { /* end directly assigned types */
				// This is not directly assigned, expect the address for it on the stack
				if elementParam.Kind == uint(reflect.Pointer) {
					targetExpressions = append(targetExpressions,
						ditypes.PrintStatement("%s", "Dereferencing pointer"),
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
						ditypes.DereferenceLocationExpression(uint(elementParam.TotalSize)),
					)
					_, ok := seenPointers[elementParam.ID]
					if !ok {
						targetExpressions = append(targetExpressions, ditypes.PopPointerAddressCompoundLocationExpression())
						seenPointers[elementParam.ID] = true
					}
				} else if elementParam.Kind == uint(reflect.Struct) {
					// Structs don't provide context on location, or have values themselves
					// Just need to copy the address for each field
					targetExpressions = append(targetExpressions,
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
						ditypes.CopyLocationExpression(),
					)

					continue
				} else if elementParam.Kind == uint(reflect.String) {
					if len(instrumentationTarget.ParameterPieces) != 2 {
						continue
					}
					stringCharArray := instrumentationTarget.ParameterPieces[0]
					stringLength := instrumentationTarget.ParameterPieces[1]
					if stringCharArray == nil || stringLength == nil {
						continue
					}

					stringLength.LocationExpressions = append(stringLength.LocationExpressions, targetExpressions...)
					if stringLength.Location != nil {
						stringLength.LocationExpressions = append(stringLength.LocationExpressions,
							ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
							ditypes.DirectReadLocationExpression(stringLength),
							ditypes.PopLocationExpression(1, 2),
						)
					} else {
						stringLength.LocationExpressions = append(stringLength.LocationExpressions,
							ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
							ditypes.ApplyOffsetLocationExpression(uint(stringLength.FieldOffset)),
							ditypes.DereferenceToOutputLocationExpression(2),
						)
					}

					targetExpressions = append(targetExpressions,
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)))

					if stringCharArray.Location != nil && stringLength.Location != nil {
						// Fields of the string are directly assigned
						targetExpressions = append(targetExpressions,
							// Read string dynamically:
							ditypes.DirectReadLocationExpression(stringCharArray),
							ditypes.DirectReadLocationExpression(stringLength),
							ditypes.DereferenceDynamicToOutputLocationExpression(uint(limitsInfo.InstrumentationOptions.StringMaxSize)),
						)
					} else {
						// Expect address of the string struct itself on the location expression stack
						targetExpressions = append(targetExpressions,
							ditypes.ReadStringToOutputLocationExpression(uint16(limitsInfo.InstrumentationOptions.StringMaxSize)),
						)
					}
					continue
					/* end parsing strings */
				} else if elementParam.Kind == uint(reflect.Slice) {
					if len(elementParam.ParameterPieces) != 3 {
						continue
					}
					slicePointer := elementParam.ParameterPieces[0]
					sliceLength := elementParam.ParameterPieces[1]

					if slicePointer == nil || sliceLength == nil {
						continue
					}

					sliceLength.LocationExpressions = append(sliceLength.LocationExpressions,
						ditypes.PrintStatement("%s", "Reading the length of slice"),
					)
					sliceLength.LocationExpressions = append(sliceLength.LocationExpressions, targetExpressions...)
					if sliceLength.Location != nil {
						sliceLength.LocationExpressions = append(sliceLength.LocationExpressions,
							ditypes.DirectReadLocationExpression(sliceLength),
							ditypes.PopLocationExpression(1, 2),
						)
					} else {
						sliceLength.LocationExpressions = append(sliceLength.LocationExpressions,
							ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
							ditypes.ApplyOffsetLocationExpression(uint(sliceLength.FieldOffset)),
							ditypes.DereferenceToOutputLocationExpression(2),
						)
					}
					if len(slicePointer.ParameterPieces) == 0 {
						continue
					}

					// Generate and collect the location expressions for collecting an individual
					// element of this slice
					sliceElementType := slicePointer.ParameterPieces[0]

					if sliceElementType == nil {
						continue
					}

					sliceIdentifier := randomLabel()
					labelName := randomLabel()

					if slicePointer.Location != nil && sliceLength.Location != nil {
						// Fields of the slice are directly assigned
						targetExpressions = append(targetExpressions,
							ditypes.PrintStatement("%s", "Reading the length of slice and setting limit (directly read)"),
							ditypes.DirectReadLocationExpression(sliceLength),
							ditypes.SetLimitEntry(sliceIdentifier, uint(ditypes.SliceMaxLength)),
						)
						for i := 0; i < ditypes.SliceMaxLength; i++ {
							GenerateLocationExpression(limitsInfo, sliceElementType)
							expressionsToUseForEachSliceElement := collectAllLocationExpressions(sliceElementType, true)
							targetExpressions = append(targetExpressions,
								ditypes.PrintStatement("%s", "Reading slice element "+fmt.Sprintf("%d", i)),
								ditypes.JumpToLabelIfEqualToLimit(uint(i), sliceIdentifier, labelName),
								ditypes.DirectReadLocationExpression(slicePointer),
								ditypes.ApplyOffsetLocationExpression(uint(sliceElementType.TotalSize)*uint(i)),
							)
							targetExpressions = append(targetExpressions, expressionsToUseForEachSliceElement...)
						}
					} else {
						// Expect address of the slice struct on stack, use offsets accordingly
						targetExpressions = append(targetExpressions,
							ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)), // Apply offset to the slice struct itself (incase we're in a struct on the stack or pointer)
							ditypes.PrintStatement("%s", "Reading the length of slice and setting limit (indirect read)"),
							ditypes.CopyLocationExpression(),         // Setup stack so it has two pointers to slice struct
							ditypes.ApplyOffsetLocationExpression(8), // Change the top pointer to the address of the length field
							ditypes.DereferenceLocationExpression(8), // Dereference to place length on top of the stack
							ditypes.SetLimitEntry(sliceIdentifier, uint(ditypes.SliceMaxLength)),
						)
						// Expect address of slice struct on top of the stack, check limit and copy/apply offset accordingly
						for i := 0; i < ditypes.SliceMaxLength; i++ {
							GenerateLocationExpression(limitsInfo, sliceElementType)
							expressionsToUseForEachSliceElement := collectAllLocationExpressions(sliceElementType, true)
							targetExpressions = append(targetExpressions,
								ditypes.PrintStatement("%s", "Reading slice element "+fmt.Sprintf("%d", i)),
								ditypes.JumpToLabelIfEqualToLimit(uint(i), sliceIdentifier, labelName),
								ditypes.CopyLocationExpression(),
								ditypes.DereferenceLocationExpression(8),
								ditypes.ApplyOffsetLocationExpression(uint(i*(int(sliceElementType.TotalSize)))),
							)
							targetExpressions = append(targetExpressions, expressionsToUseForEachSliceElement...)
						}
					}
					targetExpressions = append(targetExpressions, ditypes.InsertLabel(labelName))
					continue
					/* end parsing slices */
				} else if elementParam.Kind == uint(reflect.Array) {
					// Expect the address of the array itself on the stack
					if elementParam.TotalSize == 0 && len(elementParam.ParameterPieces) == 0 {
						continue
					}
					//FIXME: Do we need to limit lengths of arrays??
					if elementParam.ParameterPieces[0] == nil {
						continue
					}
					GenerateLocationExpression(limitsInfo, elementParam.ParameterPieces[0])
					expressionsToUseForEachArrayElement := collectAllLocationExpressions(elementParam.ParameterPieces[0], true)
					for i := range elementParam.ParameterPieces {
						targetExpressions = append(targetExpressions,
							ditypes.CopyLocationExpression(),
							ditypes.ApplyOffsetLocationExpression(uint(int(elementParam.ParameterPieces[0].TotalSize)*i)),
						)
						targetExpressions = append(targetExpressions, expressionsToUseForEachArrayElement...)
					}
				} else {
					// Basic type, indirectly assigned
					targetExpressions = append(targetExpressions,
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
						ditypes.DereferenceToOutputLocationExpression(uint(elementParam.TotalSize)))
				}
			} /* end indirectly assigned types */
		}
		expressionTargets[i].Parameter.LocationExpressions = targetExpressions
	}
}

func collectAllLocationExpressions(parameter *ditypes.Parameter, remove bool) []ditypes.LocationExpression {
	if parameter == nil {
		return []ditypes.LocationExpression{}
	}
	expressions := parameter.LocationExpressions
	for i := range parameter.ParameterPieces {
		expressions = append(expressions, collectAllLocationExpressions(parameter.ParameterPieces[i], remove)...)
	}
	if remove {
		parameter.LocationExpressions = []ditypes.LocationExpression{}
	}
	return expressions
}

type expressionParamTuple struct {
	TypePath  string
	Parameter *ditypes.Parameter
}

// generateLocationVisitsMap follows the tree of parameters (parameter.ParameterPieces), and
// collects string values of all the paths to nodes that need expressions (`needsExpressions`),
// as well as all combinations of elements that can be achieved by walking the tree (`trieKeys`).
func generateLocationVisitsMap(parameter *ditypes.Parameter) (trieKeys, needsExpressions []expressionParamTuple) {
	trieKeys = []expressionParamTuple{}
	needsExpressions = []expressionParamTuple{}

	var visit func(param *ditypes.Parameter, path string)
	visit = func(param *ditypes.Parameter, path string) {
		if param == nil {
			return
		}

		if param.DoNotCapture {
			log.Tracef("Not going to capture parameter: %s", param.Name)
			return
		}

		trieKeys = append(trieKeys, expressionParamTuple{path + param.Name + param.Type, param})

		if (len(param.ParameterPieces) == 0 ||
			isBasicType(param.Kind) ||
			param.Kind == uint(reflect.Array) ||
			param.Kind == uint(reflect.Slice)) &&
			param.Kind != uint(reflect.Struct) &&
			param.Kind != uint(reflect.Pointer) {
			needsExpressions = append(needsExpressions, expressionParamTuple{path + param.Name + param.Type, param})
			return
		}

		for i := range param.ParameterPieces {
			newPath := path + param.Name + param.Type + "@"
			visit(param.ParameterPieces[i], newPath)
		}
	}
	visit(parameter, "")
	return trieKeys, needsExpressions
}

func isBasicType(kind uint) bool {
	switch reflect.Kind(kind) {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return true
	default:
		return false
	}
}

func randomLabel() string {
	length := 6
	randomString := make([]byte, length)
	for i := 0; i < length; i++ {
		randomString[i] = byte(65 + rand.Intn(25))
	}
	return string(randomString)
}
