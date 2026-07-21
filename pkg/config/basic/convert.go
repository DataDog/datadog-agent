// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package basic

import (
	"time"

	"github.com/spf13/cast"
)

// ConvertToDefaultType casts value to the type of defaultValue. Map types are reshaped only when coerceMaps is true (used for JSON-decoded inputs).
func ConvertToDefaultType(value, defaultValue interface{}, coerceMaps bool) (interface{}, error) {
	if defaultValue == nil {
		return value, nil
	}
	switch defaultValue.(type) {
	case bool:
		return cast.ToBoolE(value)
	case string:
		return cast.ToStringE(value)
	case int, int8, int16, int32:
		return cast.ToIntE(value)
	case int64:
		return cast.ToInt64E(value)
	case uint, uint8, uint16, uint32:
		return cast.ToUintE(value)
	case uint64:
		return cast.ToUint64E(value)
	case float32, float64:
		return cast.ToFloat64E(value)
	case time.Time:
		return cast.ToTimeE(value)
	case time.Duration:
		return cast.ToDurationE(value)
	case []string:
		return cast.ToStringSliceE(value)
	case []int:
		return toNumberSliceE(value, cast.ToIntE)
	case []int32:
		return toNumberSliceE(value, cast.ToInt32E)
	case []int64:
		return toNumberSliceE(value, cast.ToInt64E)
	case []uint:
		return toNumberSliceE(value, cast.ToUintE)
	case []uint16:
		return toNumberSliceE(value, cast.ToUint16E)
	case []uint32:
		return toNumberSliceE(value, cast.ToUint32E)
	case []uint64:
		return toNumberSliceE(value, cast.ToUint64E)
	case []float32:
		return toNumberSliceE(value, cast.ToFloat32E)
	case []float64:
		return toNumberSliceE(value, cast.ToFloat64E)
	case map[string]interface{}:
		if coerceMaps {
			return cast.ToStringMapE(value)
		}
	case map[string]string:
		if coerceMaps {
			return cast.ToStringMapStringE(value)
		}
	case map[string][]string:
		if coerceMaps {
			return cast.ToStringMapStringSliceE(value)
		}
	}
	return value, nil
}

// toNumberSliceE converts value into a slice of T, applying conv to each element
func toNumberSliceE[T any](value interface{}, conv func(interface{}) (T, error)) ([]T, error) {
	// so a scalar string from env variables is split on whitespace
	raw, err := cast.ToStringSliceE(value)
	if err != nil {
		return nil, err
	}
	out := make([]T, len(raw))
	for i, v := range raw {
		n, err := conv(v)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}
