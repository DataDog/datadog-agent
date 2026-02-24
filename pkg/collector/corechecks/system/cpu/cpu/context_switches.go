// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package cpu

import "errors"

// GetContextSwitches retrieves the number of context switches for the current process.
// It returns an integer representing the count and an error if the retrieval fails.
func GetContextSwitches() (int64, error) {
	return 0, errors.New("context switches not supported on macOS")
}
