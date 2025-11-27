// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resolver

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// This is a little bit of black magic used to determine if ints and uints are
// either 32 or 64 bits on this platform. In theory they should be the same, so
// this is just for completeness.
const (
	uintBits = 32 << (^uint(0) >> 32 & 1)
	intBits  = 32 << (^int(0) >> 32 & 1)
)

type AnyValueType interface{}

func ConvertAnyValue[T AnyValueType](raw any) (T, error) {
	var (
		t     T
		out   any
		err   error
		bytes []byte
	)

	switch any(t).(type) {
	case string, fmt.Stringer, []byte:
		out = toString(raw)
	case bool:
		out, err = toBool(raw)
	case int:
		out, err = toInt(raw)
	case int64:
		out, err = toInt64(raw)
	case uint:
		out, err = toUint(raw)
	case uint64:
		out, err = toUint64(raw)
	case float64:
		out, err = toFloat64(raw)
	case time.Duration:
		out, err = toDuration(raw)
	default:
		bytes, err = json.Marshal(raw)
		if err != nil {
			return t, err
		}
		if err = json.Unmarshal(bytes, &t); err != nil {
			err = fmt.Errorf("unable to unmarshal secret to value of type %T", raw)
		}
		return t, err
	}
	if err != nil {
		return t, err
	}

	return out.(T), nil
}

func toString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toBool(val any) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("cannot convert to bool")
	}
}

func toInt(val any) (int, error) {
	switch v := val.(type) {
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	case float64:
		return int(v), nil
	case int64:
		return int(v), nil
	case uint:
		return int(v), nil
	default:
		return 0, fmt.Errorf("cannot convert to int")
	}
}

func toInt64(val any) (int64, error) {
	switch v := val.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, intBits)
	case float64:
		return int64(v), nil
	case uint:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("cannot convert to int64")
	}
}

func toUint(val any) (uint, error) {
	switch v := val.(type) {
	case uint:
		return v, nil
	case int:
		return uint(v), nil
	case string:
		u64, err := strconv.ParseUint(v, 10, uintBits)
		if err == nil {
			return uint(u64), nil
		}
		return 0, err
	default:
		return 0, fmt.Errorf("cannot convert to uint")
	}
}

func toUint64(val any) (uint64, error) {
	switch v := val.(type) {
	case uint64:
		return v, nil
	case int:
		return uint64(v), nil
	case string:
		return strconv.ParseUint(v, 10, uintBits)
	default:
		return 0, fmt.Errorf("cannot convert to uint64")
	}
}

func toFloat64(val any) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert to float64")
	}
}

func toDuration(val any) (time.Duration, error) {
	switch v := val.(type) {
	case time.Duration:
		return v, nil
	case string:
		return time.ParseDuration(v)
	case int64:
		return time.Duration(v), nil
	case int:
		return time.Duration(v), nil
	default:
		return 0, fmt.Errorf("cannot convert to time.Duration")
	}
}
