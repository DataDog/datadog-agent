package codegen

import (
	"math/rand"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
)

func flattenParameters(params []ditypes.Parameter) []ditypes.Parameter {
	flattenedParams := []ditypes.Parameter{}
	for i := range params {
		kind := reflect.Kind(params[i].Kind)
		if hasHeader(kind) {
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

func hasHeader(kind reflect.Kind) bool {
	return kind == reflect.Struct ||
		kind == reflect.Slice ||
		kind == reflect.Array ||
		kind == reflect.Pointer
}

func randomID() string {
	length := 6
	ran_str := make([]byte, length)
	for i := 0; i < length; i++ {
		ran_str[i] = byte(65 + rand.Intn(25))
	}
	return string(ran_str)
}
