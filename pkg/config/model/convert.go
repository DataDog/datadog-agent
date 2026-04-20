// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"time"

	"github.com/spf13/cast"
)

// ConvertToDefaultType casts value to the type of defaultValue; returns value unchanged if defaultValue is nil or untyped.
func ConvertToDefaultType(value interface{}, defaultValue interface{}) (interface{}, error) {
	if defaultValue == nil {
		return value, nil
	}
	switch defaultValue.(type) {
	case bool:
		return cast.ToBoolE(value)
	case string:
		return cast.ToStringE(value)
	case int32, int16, int8, int:
		return cast.ToIntE(value)
	case int64:
		return cast.ToInt64E(value)
	case float64, float32:
		return cast.ToFloat64E(value)
	case time.Time:
		return cast.ToTimeE(value)
	case time.Duration:
		return cast.ToDurationE(value)
	case []string:
		return cast.ToStringSliceE(value)
	}
	return value, nil
}
