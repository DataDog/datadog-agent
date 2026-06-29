// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package os provides additional OS utilities
package os

import (
	"github.com/shirou/gopsutil/v4/process"
)

// PidExists returns true if the pid is still alive
func PidExists(pid int) bool {
	exists, _ := process.PidExists(int32(pid))
	return exists
}
