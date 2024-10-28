// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package settings implements runtime settings and profiling
package settings

import (
	"fmt"
	"strconv"
)

// GetBool returns the bool value contained in value.
// If value is a bool, returns its value
// If value is a string, it converts "true" to true and "false" to false.
// Else, returns an error.
func GetBool(v interface{}) (bool, error) {
	// to be cautious, take care of both calls with a string (cli) or a bool (programmaticaly)
	str, ok := v.(string)
	if ok {
		// string value
		switch str {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("GetBool: bad parameter value provided: %v", str)
		}

	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("GetBool: bad parameter value provided")
	}
	return b, nil
}

// GetInt returns the integer value contained in value.
// If value is a integer, returns its value
// If value is a string, it parses the string into an integer.
// Else, returns an error.
func GetInt(v interface{}) (int, error) {
	switch v := v.(type) {
	case int:
		return v, nil
	case string:
		i, err := strconv.ParseInt(v, 10, 0)
		if err != nil {
			return 0, fmt.Errorf("GetInt: %s", err)
		}
		return int(i), nil
	default:
		return 0, fmt.Errorf("GetInt: bad parameter value provided: %v", v)
	}
}
