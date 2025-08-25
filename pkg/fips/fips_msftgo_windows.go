// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto && windows && !goexperiment.boringcrypto

// Package fips is an interface for build specific status of FIPS compliance
package fips

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// Status returns a displayable string or error of FIPS Mode of the agent build and runtime
func Status() string {
	enabled, _ := Enabled()
	if enabled {
		return "enabled"
	} else {
		return "disabled"
	}
}

// Enabled checks to see if the agent runtime environment is as expected relating to its build to be FIPS compliant. For Windows this means that FIPS mode is enabled via the Windows registry.
func Enabled() (bool, error) {
	// this is copied from how microsoft/go checks the windows registry that FIPS is enabled:
	//   https://github.com/microsoft/go/blob/d0f965f87c51211b3ea554f88e94b4c68116f5d1/eng/_util/cmd/run-builder/systemfips_windows.go#L17-L54
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false, err
	}

	enabled, enabledType, err := key.GetIntegerValue("Enabled")
	if err != nil {
		return false, err
	}

	if enabledType != registry.DWORD {
		return false, fmt.Errorf("unexpected FIPS algorithm policy Enabled key type: %v, expected: %v", enabledType, registry.DWORD)
	}

	return enabled == 1, nil
}
