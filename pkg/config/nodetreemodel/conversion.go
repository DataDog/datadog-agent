// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"strconv"
	"strings"
)

func toString(v interface{}) (string, error) {
	switch it := v.(type) {
	case int, int8, int16, int32, int64:
		num, err := toInt(v)
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatInt(int64(num), 10)
		return stringVal, nil
	case uint, uint8, uint16, uint32, uint64:
		num, err := toInt(v)
		if err != nil {
			return "", err
		}
		stringVal := strconv.FormatUint(uint64(num), 10)
		return stringVal, nil
	case float32:
		return strconv.FormatFloat(float64(it), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(it, 'f', -1, 64), nil
	case string:
		return it, nil
	}
	return "", newConversionError(v, "string")
}

func toFloat(v interface{}) (float64, error) {
	switch it := v.(type) {
	case int:
		return float64(it), nil
	case int8:
		return float64(it), nil
	case int16:
		return float64(it), nil
	case int32:
		return float64(it), nil
	case int64:
		return float64(it), nil
	case uint:
		return float64(it), nil
	case uint8:
		return float64(it), nil
	case uint16:
		return float64(it), nil
	case uint32:
		return float64(it), nil
	case uint64:
		return float64(it), nil
	case float32:
		return float64(it), nil
	case float64:
		return float64(it), nil
	}
	return 0, newConversionError(v, "float")
}

func toInt(v interface{}) (int, error) {
	switch it := v.(type) {
	case int:
		return int(it), nil
	case int8:
		return int(it), nil
	case int16:
		return int(it), nil
	case int32:
		return int(it), nil
	case int64:
		return int(it), nil
	case uint:
		return int(it), nil
	case uint8:
		return int(it), nil
	case uint16:
		return int(it), nil
	case uint32:
		return int(it), nil
	case uint64:
		return int(it), nil
	case float32:
		return int(it), nil
	case float64:
		return int(it), nil
	}
	return 0, newConversionError(v, "int")
}

func toBool(v interface{}) (bool, error) {
	switch it := v.(type) {
	case bool:
		return it, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		num, err := toInt(v)
		if err != nil {
			return false, err
		}
		return num != 0, nil
	case string:
		return convertToBool(it)
	default:
		return false, newConversionError(v, "bool")
	}
}

func toStringSlice(v interface{}) ([]string, error) { //nolint: unused // TODO: fix
	switch it := v.(type) {
	case []string:
		return it, nil
	case []interface{}:
		res := make([]string, len(it))
		for idx, item := range it {
			sItem, err := toString(item)
			if err != nil {
				return nil, err
			}
			res[idx] = sItem
		}
		return res, nil
	default:
		return nil, newConversionError(v, "slice of string")
	}
}

func toFloatSlice(v interface{}) ([]float64, error) { //nolint: unused // TODO: fix
	switch it := v.(type) {
	case []float64:
		return it, nil
	case []interface{}:
		res := make([]float64, len(it))
		for idx, item := range it {
			sItem, err := toFloat(item)
			if err != nil {
				return nil, err
			}
			res[idx] = sItem
		}
		return res, nil
	default:
		return nil, newConversionError(v, "slice float64")
	}
}

// convert a string to a bool using standard yaml constants
func convertToBool(text string) (bool, error) {
	lower := strings.ToLower(text)
	if lower == "y" || lower == "yes" || lower == "on" || lower == "true" || lower == "1" {
		return true, nil
	} else if lower == "n" || lower == "no" || lower == "off" || lower == "false" || lower == "0" {
		return false, nil
	}
	return false, newConversionError(text, "bool")
}

func newConversionError(v interface{}, expectType string) error {
	return fmt.Errorf("could not convert to %s: %v of type %T", expectType, v, v)
}

func mapInterfaceToMapString(m map[interface{}]interface{}) map[string]interface{} {
	res := make(map[string]interface{}, len(m))
	for k, v := range m {
		mk := ""
		if str, ok := k.(string); ok {
			mk = str
		} else {
			mk = fmt.Sprintf("%s", k)
		}
		res[mk] = v
	}
	return res
}
