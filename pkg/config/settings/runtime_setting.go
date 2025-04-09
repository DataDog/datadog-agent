// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package settings implements runtime settings and profiling
package settings

import (
	"fmt"
	"strconv"
	"strings"
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

// GetStrings interprets the given value as a list of string separated by the
// given separator, returns them in a strings slice.
// An error is returned if the given value can't be asserted as a string.
func GetStrings(v interface{}, sep string) ([]string, error) {
	switch v := v.(type) {
	case string:
		return strings.Split(v, sep), nil
	default:
		formated := fmt.Sprintf("%v", v)
		if len(formated) > 200 {
			formated = formated[:200] + "[...truncated]"
		}
		return []string{}, fmt.Errorf("GetStrings: bad parameter value provided: %v", formated)
	}
}
