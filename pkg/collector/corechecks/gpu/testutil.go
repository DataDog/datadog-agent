// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package gpu

import (
	"testing"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// WithGPUConfigEnabled enables the GPU check configuration for testing
// and registers a cleanup to disable it after the test completes.
func WithGPUConfigEnabled(t testing.TB) {
	t.Helper()
	pkgconfigsetup.Datadog().SetInTest("gpu.enabled", true)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetInTest("gpu.enabled", false)
	})
}
