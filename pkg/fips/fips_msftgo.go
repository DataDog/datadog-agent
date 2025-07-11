// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto && !windows && !goexperiment.boringcrypto

// Package fips is an interface for build specific status of FIPS compliance
package fips

// Status returns a displayable string or error of FIPS Mode of the agent build and runtime
func Status() string {
	enabled, _ := Enabled()
	if enabled {
		return "enabled"
	} else {
		return "disabled"
	}
}

// Enabled checks to see if the agent runtime environment is as expected relating to its build to be FIPS compliant.
// For Microsoft Go, the linux binary is built with requirefips and will not run out of FIPS Mode causing a panic if OpenSSL isn't installed and running in FIPS Mode as well
func Enabled() (bool, error) {
	return true, nil
}
