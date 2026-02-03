// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package extensions

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHooks implements ExtensionHooks interface for testing
type mockHooks struct {
	preInstallErr  error
	postInstallErr error
	preRemoveErr   error
}

func (m *mockHooks) PreInstallExtension(_ context.Context, _ string, _ string) error {
	return m.preInstallErr
}

func (m *mockHooks) PostInstallExtension(_ context.Context, _ string, _ string) error {
	return m.postInstallErr
}

func (m *mockHooks) PreRemoveExtension(_ context.Context, _ string, _ string) error {
	return m.preRemoveErr
}

// TestPackageKeyDifferentiation verifies that stable and experiment packages
// are stored with different database keys to prevent data collision.
//
// The critical logic being tested: getKey() appends "-exp" suffix for experiments.
// Without this differentiation, installing experiment extensions would overwrite
// stable extensions for the same package name.
func TestPackageKeyDifferentiation(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()

	packageName := "datadog-agent"

	// Store stable package with extension "python"
	stablePkg := dbPackage{
		Name:       packageName,
		Version:    "7.50.0",
		Extensions: map[string]struct{}{"python": {}},
	}
	err = db.SetPackage(stablePkg, false) // isExperiment=false
	require.NoError(t, err)

	// Store experiment package with extension "ruby" (same package name)
	expPkg := dbPackage{
		Name:       packageName,
		Version:    "7.51.0",
		Extensions: map[string]struct{}{"ruby": {}},
	}
	err = db.SetPackage(expPkg, true) // isExperiment=true
	require.NoError(t, err)

	// CRITICAL TEST: Verify both packages coexist independently
	// If getKey() logic is broken, one would overwrite the other

	stable, err := db.GetPackage(packageName, false)
	require.NoError(t, err)
	assert.Equal(t, "7.50.0", stable.Version, "stable package should have its own version")
	assert.Contains(t, stable.Extensions, "python", "stable extensions should be preserved")
	assert.NotContains(t, stable.Extensions, "ruby", "stable should not have experiment extensions")

	experiment, err := db.GetPackage(packageName, true)
	require.NoError(t, err)
	assert.Equal(t, "7.51.0", experiment.Version, "experiment package should have its own version")
	assert.Contains(t, experiment.Extensions, "ruby", "experiment extensions should be preserved")
	assert.NotContains(t, experiment.Extensions, "python", "experiment should not have stable extensions")
}

// TestHookErrorPropagation verifies that hook failures are properly propagated
// as errors rather than being silently ignored.
//
// This tests error handling logic: when PreRemoveExtension hook fails, the error
// must bubble up to the caller so they can handle it appropriately (retry, log, etc).
func TestHookErrorPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	expectedErr := errors.New("hook failed")
	hooks := &mockHooks{
		preRemoveErr: expectedErr,
	}

	ctx := context.Background()
	err := removeSingle(ctx, "datadog-agent", "7.50.0", "python", hooks)

	require.Error(t, err, "hook failure should return error")
	assert.Contains(t, err.Error(), "hook failed", "error should contain hook failure message")
}
