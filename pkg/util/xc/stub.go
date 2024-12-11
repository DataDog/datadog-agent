// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo || windows

package xc

import "fmt"

// GetSystemFreq grabs the system clock frequency
// NOP on cross-compiled systems
func GetSystemFreq() (int64, error) {
	return 0, fmt.Errorf("frequency unavailable")
}
