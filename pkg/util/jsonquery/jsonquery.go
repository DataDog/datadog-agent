// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

// Package jsonquery interacts with jq queries
package jsonquery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/fastjq"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheTTL      = 6 * time.Hour
	jqCachePrefix = "jq-"
)

// errStopIteration aborts a RunFunc walk once the first output has been captured.
var errStopIteration = errors.New("stop iteration")

// Parse returns an (eventually cached) compiled program to run json queries.
// The returned program is immutable and safe for concurrent use.
func Parse(q string) (*fastjq.Program, error) {
	if program, found := cache.Cache.Get(jqCachePrefix + q); found {
		return program.(*fastjq.Program), nil
	}

	program, err := fastjq.Compile(q)
	if err != nil {
		return nil, err
	}

	if err := cache.Cache.Add(jqCachePrefix+q, program, cacheTTL); err != nil {
		log.Errorf("Unable to store item in cache: %v", err)
	}
	return program, nil
}

// marshalJSON encodes object into the JSON bytes fastjq operates on. HTML escaping is
// disabled so that string values reach the query in the form the caller wrote them.
func marshalJSON(object interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}
	// Encode terminates the value with a newline that fastjq has no use for.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// runFirstOutput runs a compiled program against JSON bytes and returns its first
// output. found is false when the program yields no output at all (e.g. `empty`).
func runFirstOutput(program *fastjq.Program, jsonData []byte) (output []byte, found bool, err error) {
	err = program.RunFunc(jsonData, func(result []byte) error {
		// result is only valid for the duration of the callback, so copy it out.
		output = append(output, result...)
		found = true
		return errStopIteration
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, false, err
	}
	return output, found, nil
}

// RunSingleOutput runs a JQ query against `object` and returns the string value
// (assuming there is a single output).
//
// Strings are returned unquoted; numbers and booleans are returned as they appear in
// the input; objects and arrays are returned as JSON. A `null` output is reported as
// absent, matching the behaviour of a query that selects a missing key.
func RunSingleOutput(q string, object interface{}) (string, bool, error) {
	program, err := Parse(q)
	if err != nil {
		return "", false, err
	}

	jsonData, err := marshalJSON(object)
	if err != nil {
		return "", false, err
	}

	output, found, err := runFirstOutput(program, jsonData)
	if err != nil || !found || len(output) == 0 {
		return "", false, err
	}

	if bytes.Equal(output, []byte("null")) {
		return "", false, nil
	}

	// Unquote JSON strings so that `.foo` over {"foo":"bar"} yields bar rather than "bar".
	if output[0] == '"' {
		var value string
		if err := json.Unmarshal(output, &value); err != nil {
			return "", false, fmt.Errorf("cannot decode query output: %w", err)
		}
		return value, true, nil
	}

	return string(output), true, nil
}
