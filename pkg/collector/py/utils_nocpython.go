// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !cpython

package py

import (
	"errors"
)

// ErrNotCompiled is returned by methods when cpython is not compiled in
var ErrNotCompiled = errors.New("cpython is not compiled in")

// GetPythonInterpreterMemoryUsage stub when cpython is not compiled in
func GetPythonInterpreterMemoryUsage() ([]*PythonStats, error) {
	return nil, ErrNotCompiled
}
