// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build goexperiment.systemcrypto

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

// Enabled checks to see if the agent runtime environment is as expected relating to its build to be FIPS compliant. The binary is built with requirefips and will not run out of FIPS Mode so:
// * For Linux: if OpenSSL isn't installed and running in FIPS Mode the agent will panic
// * For Windows: if FIPS Mode is not enabled in the registry then the agent will panic
func Enabled() (bool, error) {
	// requirefips only allows for the agent to run in FIPS Mode, GOFIPS=0 will not disable it
	return true, nil
}
