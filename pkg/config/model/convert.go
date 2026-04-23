// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"time"

	"github.com/spf13/cast"
)

// ConvertToDefaultType casts value to the type of defaultValue
func ConvertToDefaultType(value interface{}, defaultValue interface{}) (interface{}, error) {
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
	case []float64:
		return toFloat64SliceE(value)
	}
	return value, nil
}

// toFloat64SliceE casts any slice to []float64
func toFloat64SliceE(value interface{}) ([]float64, error) {
	raw, err := cast.ToSliceE(value)
	if err != nil {
		return nil, err
	}
	out := make([]float64, len(raw))
	for i, v := range raw {
		f, err := cast.ToFloat64E(v)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}
