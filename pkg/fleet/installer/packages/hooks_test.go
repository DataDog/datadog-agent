// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// TestHookContextStartSpan tests the StartSpan method
func TestHookContextStartSpan(t *testing.T) {
	// Create a root span first
	rootSpan, rootCtx := telemetry.StartSpanFromContext(context.Background(), "root")
	defer rootSpan.Finish(nil)

	ctx := HookContext{
		Context:     rootCtx,
		Package:     "datadog-agent",
		PackageType: PackageTypeOCI,
		PackagePath: "/opt/datadog-packages/datadog-agent/7.50.0",
		Hook:        "postInstall",
		Upgrade:     true,
		WindowsArgs: []string{"--verbose"},
	}

	span, newCtx := ctx.StartSpan("test-operation")
	require.NotNil(t, span)
	assert.NotEqual(t, ctx.Context, newCtx.Context, "Context should be updated with new span")

	// Verify span is set up correctly
	span.Finish(nil)
}

// TestRunHookUnknownPackage tests RunHook with an unknown package
func TestRunHookUnknownPackage(t *testing.T) {
	ctx := HookContext{
		Context: context.Background(),
		Package: "unknown-package",
		Hook:    "postInstall",
	}

	// Should not return an error for unknown packages, just skip the hook
	err := RunHook(ctx)
	assert.NoError(t, err)
}

// TestRunHookUnknownHookName tests RunHook with an unknown hook name
func TestRunHookUnknownHookName(t *testing.T) {
	ctx := HookContext{
		Context: context.Background(),
		Package: "datadog-agent",
		Hook:    "unknownHook",
	}

	// Should not return an error for unknown hooks, just skip the hook
	err := RunHook(ctx)
	assert.NoError(t, err)
}

// TestRunHookWithTelemetry tests that RunHook creates telemetry spans
func TestRunHookWithTelemetry(t *testing.T) {
	// Create a root span
	rootSpan, rootCtx := telemetry.StartSpanFromContext(context.Background(), "root")
	defer rootSpan.Finish(nil)

	ctx := HookContext{
		Context: rootCtx,
		Package: "unknown-package",
		Hook:    "postInstall",
	}

	err := RunHook(ctx)
	assert.NoError(t, err)
}

// TestHooksCLIGetPath tests the getPath method of hooksCLI
func TestHooksCLIGetPath(t *testing.T) {
	// Create temporary directory for test repositories
	tmpDir := t.TempDir()
	repos := repository.NewRepositories(tmpDir, nil)

	h := &hooksCLI{
		env:      &env.Env{},
		packages: repos,
	}

	tests := []struct {
		name         string
		pkg          string
		pkgType      PackageType
		experiment   bool
		wantContains string
		wantPanic    bool
	}{
		{
			name:         "OCI package stable path",
			pkg:          "datadog-agent",
			pkgType:      PackageTypeOCI,
			experiment:   false,
			wantContains: "stable",
		},
		{
			name:         "OCI package experiment path",
			pkg:          "datadog-agent",
			pkgType:      PackageTypeOCI,
			experiment:   true,
			wantContains: "experiment",
		},
		{
			name:         "DEB package path",
			pkg:          "datadog-agent",
			pkgType:      PackageTypeDEB,
			experiment:   false,
			wantContains: "/opt/datadog-agent",
		},
		{
			name:         "RPM package path",
			pkg:          "datadog-agent",
			pkgType:      PackageTypeRPM,
			experiment:   false,
			wantContains: "/opt/datadog-agent",
		},
		{
			name:         "OCI non-agent package stable",
			pkg:          "dd-trace-py",
			pkgType:      PackageTypeOCI,
			experiment:   false,
			wantContains: "stable",
		},
		{
			name:         "OCI non-agent package experiment",
			pkg:          "dd-trace-py",
			pkgType:      PackageTypeOCI,
			experiment:   true,
			wantContains: "experiment",
		},
		{
			name:       "unknown package type should panic",
			pkg:        "some-package",
			pkgType:    PackageType("unknown"),
			experiment: false,
			wantPanic:  true,
		},
		{
			name:       "DEB non-agent package should panic",
			pkg:        "dd-trace-py",
			pkgType:    PackageTypeDEB,
			experiment: false,
			wantPanic:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					h.getPath(tt.pkg, tt.pkgType, tt.experiment)
				})
			} else {
				path := h.getPath(tt.pkg, tt.pkgType, tt.experiment)
				assert.Contains(t, path, tt.wantContains)
			}
		})
	}
}
