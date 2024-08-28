// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package dogstatsd

import "testing"

// noop version for unsupported platforms
func testUDSOriginDetection(t *testing.T, _ string) {
	t.Log("Unsupported platform, skip...")
}
