// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows
// +build !linux,!windows

package config

import "fmt"

const (
	defaultConfigDir          = ""
	defaultSystemProbeAddress = ""
)

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockPath string) error {
	return fmt.Errorf("system-probe unsupported")
}
