// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

// Package python implements the layer to interact with the Python interpreter.
package python

// GetPythonInfo returns the info string as provided by the embedded Python interpreter which is "n/a" because the agent was built without python.
func GetPythonInfo() string {
	return "n/a"
}

// GetPythonVersion returns nothing here because the agent was built without python.
func GetPythonVersion() string {
	return GetPythonInfo()
}
