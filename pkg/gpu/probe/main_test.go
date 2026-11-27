// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"os"
	"testing"
)

// TestMain defined to run initialization before any test is run
func TestMain(m *testing.M) {
	memPools.ensureInitNoTelemetry()
	os.Exit(m.Run())
}
