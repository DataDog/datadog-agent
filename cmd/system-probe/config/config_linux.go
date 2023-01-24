// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package config

import (
	"fmt"
	"path/filepath"
)

const (
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"

	defaultConfigDir = "/etc/datadog-agent"
)

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockPath string) error {
	if !filepath.IsAbs(sockPath) {
		return fmt.Errorf("socket path must be an absolute file path: %s", sockPath)
	}
	return nil
}

// Right now, no, because there isn't a way to consent to closed source on Linux
// and there aren't any closed source components
func isClosedSourceAllowed() bool {
	return false
}
