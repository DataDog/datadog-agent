// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !goexperiment.boringcrypto

// Package fips is an interface for build specific status of FIPS compliance
package fips

import "crypto/fips140"

// Status returns a displayable string of the FIPS mode of the agent build and runtime.
func Status() string {
	enabled, _ := Enabled()
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// Enabled reports whether the agent runtime environment is operating in FIPS mode,
// using the standard library's fips140 package (Go 1.25+).
func Enabled() (bool, error) {
	return fips140.Enabled(), nil
}
