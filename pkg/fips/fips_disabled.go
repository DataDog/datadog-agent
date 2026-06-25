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

// BuiltForFIPS reports whether the binary was built as the FIPS flavor. Unlike
// Enabled, this is a compile-time fact and does not depend on whether the FIPS
// crypto backend is active in the current process. Use it for FIPS-flavor
// decisions (such as selecting the FIPS package variant) that must hold even
// when Enabled would momentarily report false. It is always false for the
// non-FIPS flavor.
func BuiltForFIPS() bool {
	return false
}
