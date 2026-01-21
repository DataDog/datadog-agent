// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostInstallAPMInjector_DirectMode(t *testing.T) {
	// Set environment to direct mode
	t.Setenv("DD_APM_INJECTOR_MODE", env.APMInjectorModeDirect)

	ctx := HookContext{
		Context:     context.Background(),
		Package:     "datadog-apm-inject",
		PackageType: PackageTypeOCI,
		PackagePath: "/test/path",
		Hook:        "postInstall",
		Upgrade:     false,
	}

	// This test will verify that postInstallAPMInjector doesn't panic
	// In a real environment, it would call installer.Setup()
	// We can't fully test without mocking the entire InjectorInstaller
	// So this is more of a smoke test
	err := postInstallAPMInjector(ctx)

	// We expect an error because we're not in a proper installation environment
	// but the function should not panic
	if err != nil {
		t.Logf("postInstallAPMInjector returned error (expected in test environment): %v", err)
	}
}

func TestPostInstallAPMInjector_ServiceMode(t *testing.T) {
	// Set environment to service mode
	t.Setenv("DD_APM_INJECTOR_MODE", env.APMInjectorModeService)

	ctx := HookContext{
		Context:     context.Background(),
		Package:     "datadog-apm-inject",
		PackageType: PackageTypeOCI,
		PackagePath: "/test/path",
		Hook:        "postInstall",
		Upgrade:     false,
	}

	// This test will verify that postInstallAPMInjector doesn't panic
	// In service mode, it should try to create systemd service
	err := postInstallAPMInjector(ctx)

	// We expect an error because we're not in a proper installation environment
	// but the function should not panic
	if err != nil {
		t.Logf("postInstallAPMInjector returned error (expected in test environment): %v", err)
	}
}

func TestPreRemoveAPMInjector_DirectMode(t *testing.T) {
	// Set environment to direct mode
	t.Setenv("DD_APM_INJECTOR_MODE", env.APMInjectorModeDirect)

	ctx := HookContext{
		Context:     context.Background(),
		Package:     "datadog-apm-inject",
		PackageType: PackageTypeOCI,
		PackagePath: "/test/path",
		Hook:        "preRemove",
		Upgrade:     false,
	}

	err := preRemoveAPMInjector(ctx)

	// We expect an error because we're not in a proper installation environment
	if err != nil {
		t.Logf("preRemoveAPMInjector returned error (expected in test environment): %v", err)
	}
}

func TestPreRemoveAPMInjector_ServiceMode(t *testing.T) {
	// Set environment to service mode
	t.Setenv("DD_APM_INJECTOR_MODE", env.APMInjectorModeService)

	ctx := HookContext{
		Context:     context.Background(),
		Package:     "datadog-apm-inject",
		PackageType: PackageTypeOCI,
		PackagePath: "/test/path",
		Hook:        "preRemove",
		Upgrade:     false,
	}

	err := preRemoveAPMInjector(ctx)

	// We expect an error because we're not in a proper installation environment
	if err != nil {
		t.Logf("preRemoveAPMInjector returned error (expected in test environment): %v", err)
	}
}

func TestInstrumentAPMInjector(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "instrument all",
			method: env.APMInstrumentationEnabledAll,
		},
		{
			name:   "instrument host",
			method: env.APMInstrumentationEnabledHost,
		},
		{
			name:   "instrument docker",
			method: env.APMInstrumentationEnabledDocker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := InstrumentAPMInjector(ctx, tt.method)

			// We expect errors in test environment
			// This is mainly a smoke test to ensure no panics
			if err != nil {
				t.Logf("InstrumentAPMInjector returned error (expected in test environment): %v", err)
			}
		})
	}
}

func TestUninstrumentAPMInjector(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "uninstrument all",
			method: env.APMInstrumentationEnabledAll,
		},
		{
			name:   "uninstrument host",
			method: env.APMInstrumentationEnabledHost,
		},
		{
			name:   "uninstrument docker",
			method: env.APMInstrumentationEnabledDocker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := UninstrumentAPMInjector(ctx, tt.method)

			// We expect errors in test environment
			// This is mainly a smoke test to ensure no panics
			if err != nil {
				t.Logf("UninstrumentAPMInjector returned error (expected in test environment): %v", err)
			}
		})
	}
}

func TestAPMDebRPMPackages(t *testing.T) {
	// Verify all expected packages are in the list
	expectedPackages := []string{
		"datadog-apm-inject",
		"datadog-apm-library-all",
		"datadog-apm-library-dotnet",
		"datadog-apm-library-js",
		"datadog-apm-library-java",
		"datadog-apm-library-python",
		"datadog-apm-library-ruby",
	}

	assert.Equal(t, len(expectedPackages), len(apmDebRPMPackages))

	for _, pkg := range expectedPackages {
		assert.Contains(t, apmDebRPMPackages, pkg)
	}
}

func TestAPMInjectPackageHooks(t *testing.T) {
	// Verify hooks are properly configured
	require.NotNil(t, apmInjectPackage.preInstall)
	require.NotNil(t, apmInjectPackage.postInstall)
	require.NotNil(t, apmInjectPackage.preRemove)
}
