// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

import (
	"math/rand"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

func flattenParameters(params []*ditypes.Parameter) []*ditypes.Parameter {
	flattenedParams := []*ditypes.Parameter{}
	for i := range params {
		if params[i] == nil {
			continue
		}
		kind := reflect.Kind(params[i].Kind)
		if kind == reflect.Slice || kind == reflect.String {
			// Slices don't get flattened as we need the underlying type.
			// We populate the slice's template using that type.
			//
			// Strings also don't get flattened as we need the underlying length.
			flattenedParams = append(flattenedParams, params[i])
		} else if hasHeader(kind) {
			paramHeader := params[i]
			flattenedParams = append(flattenedParams, paramHeader)
			flattenedParams = append(flattenedParams, flattenParameters(params[i].ParameterPieces)...)
			paramHeader.ParameterPieces = nil
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
