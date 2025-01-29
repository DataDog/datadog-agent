// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto && !windows

// Package fips is an interface for build specific status of FIPS compliance
package fips

import (
	"os"
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

// Enabled checks to see if the agent runtime environment is as expected relating to its build to be FIPS compliant. For Linux this is that the binary is run with the GOFIPS=1 environment variable.
func Enabled() (bool, error) {
	return os.Getenv("GOFIPS") == "1", nil
}
