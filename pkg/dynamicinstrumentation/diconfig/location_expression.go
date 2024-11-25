// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/exp/rand"
)

// GenerateLocationExpression takes metadata about a parameter, including its type and location, and generates a list of
// LocationExpressions that can be used to read the parameter from the target process.
//
// It walks the tree of the parameter and its pieces, generating LocationExpressions for each piece.
// The following logic is applied:
func GenerateLocationExpression(limitsInfo *ditypes.InstrumentationInfo, param *ditypes.Parameter) []ditypes.LocationExpression {
	expressions := []ditypes.LocationExpression{}
	triePaths, expressionTargets := generateLocationVisitsMap(param)
	// Go through each target type/field which needs to be captured
	for pathToInstrumentationTarget, instrumentationTarget := range expressionTargets {
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
		for i := range pathElements {
			elementParam, ok := triePaths[pathElements[i]]
			if !ok {
				log.Infof("Path not found to target: %s", pathElements[i])
				continue
			}

			// Check if this instrumentation target is directly assigned
			if elementParam.Location != nil {
				// Type is directly assigned
				if elementParam.Kind == uint(reflect.Array) {
					if elementParam.TotalSize == 0 && len(elementParam.ParameterPieces) == 0 {
						continue
					}
					GenerateLocationExpression(limitsInfo, &elementParam.ParameterPieces[0])
					expressionsToUseForEachArrayElement := collectAndRemoveLocationExpressions(&elementParam.ParameterPieces[0])
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
				} else {
					targetExpressions = append(targetExpressions, ditypes.DirectReadLocationExpression(elementParam))
					if elementParam.Kind != uint(reflect.Pointer) {
						// Since this isn't a pointer, we can just directly read
						targetExpressions = append(targetExpressions, ditypes.PopLocationExpression(1, uint(elementParam.TotalSize)))
					}
				}
				continue
				/* end directly assigned types */
			} else {
				// This is not directly assigned, expect the address for it on the stack
				if elementParam.Kind == uint(reflect.Pointer) {
					targetExpressions = append(targetExpressions,
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
						ditypes.DereferenceLocationExpression(uint(elementParam.TotalSize)),
					)
				} else if elementParam.Kind == uint(reflect.Struct) {
					// Structs don't provide context on location, or have values themselves
					continue
				} else if elementParam.Kind == uint(reflect.String) {
					if len(instrumentationTarget.ParameterPieces) != 2 {
						continue
					}
					str := instrumentationTarget.ParameterPieces[0]
					len := instrumentationTarget.ParameterPieces[1]
					if str.Location != nil && len.Location != nil {
						// Fields of the string are directly assigned
						targetExpressions = append(targetExpressions,
							// Read length to output buffer:
							ditypes.DirectReadLocationExpression(&len),
							ditypes.PopLocationExpression(1, 2),
							// Read string dynamically:
							ditypes.DirectReadLocationExpression(&str),
							ditypes.DirectReadLocationExpression(&len),
							ditypes.DereferenceDynamicToOutputLocationExpression(uint(limitsInfo.InstrumentationOptions.StringMaxSize)),
						)
					} else {
						// Expect address of the string struct itself on the location expression stack
						targetExpressions = append(targetExpressions,
							ditypes.ReadStringToOutput(uint16(limitsInfo.InstrumentationOptions.StringMaxSize)),
						)
					}
					continue
					/* end parsing strings */
				} else if elementParam.Kind == uint(reflect.Slice) {
					if len(elementParam.ParameterPieces) != 3 {
						continue
					}
					ptr := elementParam.ParameterPieces[0]
					len := elementParam.ParameterPieces[1]
					sliceElementType := ptr.ParameterPieces[0]

					// Generate and collect the location expressions for collecting an individual
					// element of this slice
					GenerateLocationExpression(limitsInfo, &ptr.ParameterPieces[0])
					expressionsToUseForEachSliceElement := collectAndRemoveLocationExpressions(&ptr.ParameterPieces[0])

					labelName := randomLabel()
					if ptr.Location != nil && len.Location != nil {
						// Fields of the slice are directly assigned
						len.TotalSize = 2
						targetExpressions = append(targetExpressions,
							// Read slice length to output buffer:
							ditypes.DirectReadLocationExpression(&len),
							ditypes.PopLocationExpression(1, 2),
							ditypes.DirectReadLocationExpression(&len),
							ditypes.SetGlobalLimitVariable(uint(ditypes.SliceMaxLength)),
						)
						for i := 0; i < ditypes.SliceMaxLength; i++ {
							targetExpressions = append(targetExpressions,
								ditypes.JumpToLabelIfEqualToLimit(uint(i), labelName),
								// Read the slice element:
								ditypes.DirectReadLocationExpression(&ptr),
								ditypes.ApplyOffsetLocationExpression(uint(sliceElementType.TotalSize)*uint(i)),
							)
							targetExpressions = append(targetExpressions, expressionsToUseForEachSliceElement...)
						}
					} else {
						// Expect address of the slice struct on stack, use offsets accordingly
						targetExpressions = append(targetExpressions,
							ditypes.CopyLocationExpression(),         // Setup stack so it has two pointers to slice struct
							ditypes.ApplyOffsetLocationExpression(8), // Change the top pointer to the address of the length field
							ditypes.DereferenceLocationExpression(8), // Dereference to place length on top of the stack
							ditypes.CopyLocationExpression(),         // Copy the length so it can be used for setting the global limit, and being sent to output
							ditypes.SetGlobalLimitVariable(uint(ditypes.SliceMaxLength)),
							ditypes.PopLocationExpression(1, 2),
						)
						// Pointer to slice struct on stack
						for i := 0; i < ditypes.SliceMaxLength; i++ {
							targetExpressions = append(targetExpressions,
								ditypes.JumpToLabelIfEqualToLimit(uint(i), labelName),
								ditypes.CopyLocationExpression(),
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
					GenerateLocationExpression(limitsInfo, &elementParam.ParameterPieces[0])
					expressionsToUseForEachArrayElement := collectAndRemoveLocationExpressions(&elementParam.ParameterPieces[0])
					for i := 0; i < len(elementParam.ParameterPieces); i++ {
						targetExpressions = append(targetExpressions,
							ditypes.CopyLocationExpression(),
							ditypes.ApplyOffsetLocationExpression(uint(int(elementParam.ParameterPieces[0].TotalSize)*i)),
						)
						targetExpressions = append(targetExpressions, expressionsToUseForEachArrayElement...)
					}
				} else {
					targetExpressions = append(targetExpressions,
						ditypes.ApplyOffsetLocationExpression(uint(elementParam.FieldOffset)),
						ditypes.DereferenceToOutputLocationExpression(uint(elementParam.TotalSize)))
				}
			}
		}
		expressions = append(expressions, targetExpressions...)
	}
	return expressions
}

// collectAndRemoveLocationExpressions goes through the parameter tree (param.ParameterPieces) via
// depth first traversal, collecting the LocationExpression's from each parameter and appending them
// to a collective slice. As it collects the location expressions, it removes them from that parameter.
func collectAndRemoveLocationExpressions(param *ditypes.Parameter) []ditypes.LocationExpression {
	collectedExpressions := []ditypes.LocationExpression{}
	queue := []*ditypes.Parameter{param}
	var top *ditypes.Parameter

	for {
		if len(queue) == 0 {
			break
		}
		top = queue[0]
		queue = queue[1:]
		for i := range top.ParameterPieces {
			queue = append(queue, &top.ParameterPieces[i])
		}
		if len(top.LocationExpressions) > 0 {
			collectedExpressions = append(top.LocationExpressions, collectedExpressions...)
			top.LocationExpressions = []ditypes.LocationExpression{}
		}
	}
	return collectedExpressions
}

// generateLocationVisitsMap follows the tree of parameters (parameter.ParameterPieces), and
// collects string values of all the paths to nodes that need expressions (`needsExpressions`),
// as well as all combinations of elements that can be achieved by walking the tree (`trieKeys`).
func generateLocationVisitsMap(parameter *ditypes.Parameter) (trieKeys map[string]*ditypes.Parameter, needsExpressions map[string]*ditypes.Parameter) {
	trieKeys = map[string]*ditypes.Parameter{}
	needsExpressions = map[string]*ditypes.Parameter{}

	var visit func(param *ditypes.Parameter, path string)
	visit = func(param *ditypes.Parameter, path string) {
		trieKeys[path+param.Type] = param

		if (len(param.ParameterPieces) == 0 ||
			isBasicType(param.Kind) ||
			param.Kind == uint(reflect.Array) ||
			param.Kind == uint(reflect.Slice)) &&
			param.Kind != uint(reflect.Struct) &&
			param.Kind != uint(reflect.Pointer) {
			needsExpressions[path+param.Type] = param
			return
		}

		for i := range param.ParameterPieces {
			newPath := path + param.Type + "@"
			visit(&param.ParameterPieces[i], newPath)
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
