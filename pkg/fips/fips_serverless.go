// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverlessfips

// Package fips is an interface for build specific status of FIPS compliance
package fips

// This is the main thing. In conjunction with the boringcrypto experiment tag
// and Cgo, this ensures that the agent is built in a FIPS compliant way. We do
// this in the datadog-lambda-extension build tooling.
import _ "crypto/tls/fipsonly"

// Status returns a displayable string or error of FIPS Mode of the agent build and runtime
func Status() string {
	enabled, _ := Enabled()
	if enabled {
		return "enabled"
	} else {
		return "disabled"
	}
}

// Enabled checks to see if the agent runtime environment is as expected
// relating to its build to be FIPS compliant. The serverless binary is built
// with serverlessfips and our build tooling in datadog-lambda-extension will
// fail if boringcrypto is not set up with fipsonly.
func Enabled() (bool, error) {
	return true, nil
}

