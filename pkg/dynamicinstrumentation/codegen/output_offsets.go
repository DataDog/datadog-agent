// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"math/rand"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type paramDepthCounter struct {
	depth int
	param *ditypes.Parameter
}

func applyCaptureDepth(params []ditypes.Parameter, maxDepth int) []ditypes.Parameter {
	log.Tracef("Applying capture depth: %d", maxDepth)
	queue := []paramDepthCounter{}

	for i := range params {
		queue = append(queue, paramDepthCounter{
			depth: 0,
			param: &params[i],
		})
	}

	for len(queue) != 0 {
		front := queue[0]
		queue = queue[1:]

		if front.depth == maxDepth {
			// max capture depth reached, remove parameters below this level.
			front.param.ParameterPieces = []ditypes.Parameter{}
			if front.param.Kind == uint(reflect.Struct) {
				// struct size reflects the number of fields,
				// setting to 0 tells the user space parsing not to
				// expect anything else.
				front.param.TotalSize = 0
			}
		} else {
			for i := range front.param.ParameterPieces {
				queue = append(queue, paramDepthCounter{
					depth: front.depth + 1,
					param: &front.param.ParameterPieces[i],
				})
			}
		}
	}
	return params
}

func flattenParameters(params []ditypes.Parameter) []ditypes.Parameter {
	flattenedParams := []ditypes.Parameter{}
	for i := range params {
		kind := reflect.Kind(params[i].Kind)
		if kind == reflect.Slice {
			// Slices don't get flattened as we need the underlying type.
			// We populate the slice's template using that type.
			flattenedParams = append(flattenedParams, params[i])
		} else if hasHeader(kind) {
			paramHeader := params[i]
			paramHeader.ParameterPieces = nil
			flattenedParams = append(flattenedParams, paramHeader)
			flattenedParams = append(flattenedParams, flattenParameters(params[i].ParameterPieces)...)
		} else if len(params[i].ParameterPieces) > 0 {
			flattenedParams = append(flattenedParams, flattenParameters(params[i].ParameterPieces)...)
		} else {
			flattenedParams = append(flattenedParams, params[i])
		}
	}

	for i := range flattenedParams {
		flattenedParams[i].ID = randomID()
	}

	return flattenedParams
}

func applyFieldCountLimit(params []ditypes.Parameter) {
	queue := []*ditypes.Parameter{}
	for i := range params {
		queue = append(queue, &params[len(params)-1-i])
	}
	var (
		current *ditypes.Parameter
		max     int
	)
	for len(queue) != 0 {
		current = queue[0]
		queue = queue[1:]

		max = len(current.ParameterPieces)
		if len(current.ParameterPieces) > ditypes.MaxFieldCount {
			max = ditypes.MaxFieldCount
			for j := max; j < len(current.ParameterPieces); j++ {
				excludeForFieldCount(&current.ParameterPieces[j])
			}
		}
		for n := 0; n < max; n++ {
			queue = append(queue, &current.ParameterPieces[n])
		}
	}
}

func excludeForFieldCount(root *ditypes.Parameter) {
	// Exclude all in this tree
	if root == nil {
		return
	}
	root.NotCaptureReason = ditypes.FieldLimitReached
	root.Kind = ditypes.KindCutFieldLimit
	for i := range root.ParameterPieces {
		excludeForFieldCount(&root.ParameterPieces[i])
	}
}

func hasHeader(kind reflect.Kind) bool {
	return kind == reflect.Struct ||
		kind == reflect.Array ||
		kind == reflect.Pointer
}

func randomID() string {
	length := 6
	randomString := make([]byte, length)
	for i := 0; i < length; i++ {
		randomString[i] = byte(65 + rand.Intn(25))
	}
	return string(randomString)
}
