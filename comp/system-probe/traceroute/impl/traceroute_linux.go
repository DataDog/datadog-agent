// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracerouteimpl

// startPlatformDriver is a no-op on Linux
func startPlatformDriver() error {
	// No driver needed on Linux
	return nil
}

// stopPlatformDriver is a no-op on Linux
func stopPlatformDriver() error {
	// No driver needed on Linux
	return nil
}
