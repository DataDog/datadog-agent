// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package ports

import "path/filepath"

// RetrieveProcessName returns the base name of the process on non-windows systems
func RetrieveProcessName(_ int, processName string) (string, error) {
	return filepath.Base(processName), nil
}
