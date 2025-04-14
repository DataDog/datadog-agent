// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen is used to generate bpf program source code based on probe definitions
package codegen

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type captureDepthItem struct {
	depth     int
	parameter *ditypes.Parameter
}

func applyExclusions(params []*ditypes.Parameter) []*ditypes.Parameter {
	params = pruneDoNotCaptureParams(params)
	correctStructSizes(params)
	return params
}

// pruneDoNotCaptureParams removes any parameters with DoNotCapture set to true from the parameter tree
func pruneDoNotCaptureParams(params []*ditypes.Parameter) []*ditypes.Parameter {
	if len(params) == 0 {
		return params
	}

	result := []*ditypes.Parameter{}
	for _, param := range params {
		if param == nil {
			continue
		}
		if param.DoNotCapture && param.Kind != uint(reflect.Pointer) {
			continue
		}

		// Recursively prune child parameters
		if len(param.ParameterPieces) > 0 {
			param.ParameterPieces = pruneDoNotCaptureParams(param.ParameterPieces)
		}

		result = append(result, param)
	}

	return result
}

func dontCaptureInterfaces(params []*ditypes.Parameter) {
	if len(params) == 0 {
		return
	}

	// Create a queue to hold parameters that need to be processed
	queue := make([]*ditypes.Parameter, 0, len(params))

	// Initialize the queue with the top-level parameters
	for _, param := range params {
		if param != nil {
			queue = append(queue, param)
		}
	}

	// Process parameters until the queue is empty
	for len(queue) > 0 {
		// Dequeue the next parameter
		param := queue[0]
		queue = queue[1:]

		if param == nil {
			continue
		}

		// Check if the parameter is an interface type
		if param.Type == "runtime.iface" {
			param.DoNotCapture = true
			param.NotCaptureReason = ditypes.Unsupported
		}

		// Add nested parameters to the queue
		for _, childParam := range param.ParameterPieces {
			if childParam != nil {
				queue = append(queue, childParam)
			}
		}
	}
}

func setFieldLimit(params []*ditypes.Parameter, fieldCountLimit int) {
	if fieldCountLimit <= 0 || len(params) == 0 {
		return
	}

	// Create a queue to hold parameters that need to be processed
	queue := make([]*ditypes.Parameter, 0, len(params))

	// Initialize the queue with the top-level parameters
	for _, param := range params {
		if param != nil {
			queue = append(queue, param)
		}
	}

	// Process parameters until the queue is empty
	for len(queue) > 0 {
		// Dequeue the next parameter
		param := queue[0]
		queue = queue[1:]

		// Apply field limiting to struct types
		if reflect.Kind(param.Kind) == reflect.Struct {
			markExcessiveFieldsDoNotCapture(param, fieldCountLimit)
		}

		// Add nested parameters to the queue
		for _, childParam := range param.ParameterPieces {
			if childParam != nil {
				queue = append(queue, childParam)
			}
		}
	}
}

// markExcessiveFieldsDoNotCapture sets DoNotCapture=true for fields beyond the limit in a struct
func markExcessiveFieldsDoNotCapture(structParam *ditypes.Parameter, fieldCountLimit int) {
	if structParam == nil || len(structParam.ParameterPieces) <= fieldCountLimit {
		return
	}

	// Mark fields beyond the limit as DoNotCapture
	for i := fieldCountLimit; i < len(structParam.ParameterPieces); i++ {
		if structParam.ParameterPieces[i] != nil {
			structParam.ParameterPieces[i].DoNotCapture = true
			structParam.ParameterPieces[i].NotCaptureReason = ditypes.FieldLimitReached
		}
	}
}

// setDepthLimit sets the DoNotCapture flag on all parameters that are at or beyond the target depth
func setDepthLimit(params []*ditypes.Parameter, targetDepth int) {
	queue := []*captureDepthItem{}
	for i := range params {
		if params[i] == nil {
			continue
		}
		queue = append(queue, &captureDepthItem{
			depth:     1,
			parameter: params[i],
		})
	}

	for len(queue) > 0 {
		top := queue[0]
		queue = queue[1:]

		if top.parameter == nil {
			continue
		}

		if top.depth > targetDepth {
			top.parameter.DoNotCapture = true
			top.parameter.NotCaptureReason = ditypes.CaptureDepthReached
		}

		switch reflect.Kind(top.parameter.Kind) {
		case reflect.Struct:
			fallthrough
		case reflect.Array:
			if top.depth+1 > targetDepth {
				top.parameter.DoNotCapture = true
				top.parameter.NotCaptureReason = ditypes.CaptureDepthReached
			}
			for _, child := range top.parameter.ParameterPieces {
				queue = append(queue, &captureDepthItem{depth: top.depth + 1, parameter: child})
			}

		case reflect.Slice:
			// If we do want to capture the slice, it means that we at least capture the first
			// layer of the slice elements. Only this layer counts towards capture depth, but
			// if it's set to DoNotCapture, then this slice layer header should be set to it as well
			if len(top.parameter.ParameterPieces) == 0 || top.parameter.ParameterPieces[0] == nil ||
				len(top.parameter.ParameterPieces[0].ParameterPieces) == 0 ||
				top.parameter.ParameterPieces[0].ParameterPieces[0] == nil {
				continue
			}
			if top.depth+1 > targetDepth {
				top.parameter.DoNotCapture = true
				top.parameter.NotCaptureReason = ditypes.CaptureDepthReached
			}
			elementType := top.parameter.ParameterPieces[0].ParameterPieces[0]
			queue = append(queue, &captureDepthItem{depth: top.depth + 1, parameter: elementType})

		case reflect.Pointer:
			if len(top.parameter.ParameterPieces) == 0 {
				continue
			}
			valueType := top.parameter.ParameterPieces[0]
			queue = append(queue, &captureDepthItem{depth: top.depth, parameter: valueType})

		case reflect.String:
			// Propagate DoNotCapture/NotCaptureReason value to string fields (char*, len) for clarity
			if len(top.parameter.ParameterPieces) == 2 &&
				top.parameter.ParameterPieces[0] != nil &&
				len(top.parameter.ParameterPieces[0].ParameterPieces) == 1 &&
				top.parameter.ParameterPieces[0].ParameterPieces[0] != nil {

				top.parameter.ParameterPieces[0].DoNotCapture = top.parameter.DoNotCapture
				top.parameter.ParameterPieces[0].NotCaptureReason = top.parameter.NotCaptureReason
				top.parameter.ParameterPieces[0].ParameterPieces[0].DoNotCapture = top.parameter.DoNotCapture
				top.parameter.ParameterPieces[0].ParameterPieces[0].NotCaptureReason = top.parameter.NotCaptureReason
				top.parameter.ParameterPieces[1].DoNotCapture = top.parameter.DoNotCapture
				top.parameter.ParameterPieces[1].NotCaptureReason = top.parameter.NotCaptureReason
			}
			continue

		default:
			for _, child := range top.parameter.ParameterPieces {
				queue = append(queue, &captureDepthItem{depth: top.depth + 1, parameter: child})
			}
		}
	}
}

// correctStructSizes sets the size of all passed struct parameters to the number of fields in the struct
func correctStructSizes(params []*ditypes.Parameter) {
	for i := range params {
		correctStructSize(params[i])
	}
}

// correctStructSize sets the size of struct parameters to the number of fields in the struct
func correctStructSize(param *ditypes.Parameter) {
	if param == nil || len(param.ParameterPieces) == 0 {
		return
	}
	if param.Kind == uint(reflect.Struct) || param.Kind == uint(reflect.Array) {
		param.TotalSize = int64(len(param.ParameterPieces))
	}
	for i := range param.ParameterPieces {
		correctStructSize(param.ParameterPieces[i])
	}
}

// correctPointersWithoutPieces checks recursively if any parameter of kind Pointer in the parameter tree
// has no parameter pieces and corrects it accordingly. This can occur for various reasons, such as
// when capture depth is enforced.
func correctPointersWithoutPieces(params []*ditypes.Parameter) {
	if len(params) == 0 {
		return
	}

	queue := make([]*ditypes.Parameter, 0, len(params))
	for _, param := range params {
		if param != nil {
			queue = append(queue, param)
		}
	}

	for len(queue) > 0 {
		param := queue[0]
		queue = queue[1:]
		if param == nil {
			continue
		}

		// Check if parameter is a pointer with no parameter pieces
		if reflect.Kind(param.Kind) == reflect.Pointer && len(param.ParameterPieces) == 0 {
			param.ParameterPieces = []*ditypes.Parameter{{}}
			return
		}
		for _, childParam := range param.ParameterPieces {
			if childParam != nil {
				queue = append(queue, childParam)
			}
		}
	}
	return
}
