// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !test

package util

// SetAuthTokenInMemory is only expected to be used for unit-tests
// In a non-test environment, this function is a no-op
func SetAuthTokenInMemory() {
	// No-op implementation
}
