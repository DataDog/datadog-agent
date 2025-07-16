// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !goexperiment.systemcrypto && !goexperiment.boringcrypto

// Package fips is an interface for build specific status of FIPS compliance
package fips

// Status returns an empty string when not the datadog-fips-agent flavor
func Status() string {
	return "not available"
}

// Enabled  returns false when not the datadog-fips-agent flavor
func Enabled() (bool, error) {
	return false, nil
}
