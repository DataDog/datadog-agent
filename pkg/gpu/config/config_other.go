// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package config

import "errors"

// CheckGPUSupported checks if the host's kernel supports GPU monitoring
func CheckGPUSupported() error {
	return errors.New("GPU monitoring is not supported on this platform")
}
