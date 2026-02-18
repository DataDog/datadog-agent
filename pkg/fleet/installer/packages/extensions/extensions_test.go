// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package extensions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestSaveWritesExtensionList verifies that Save writes the list of installed
// extensions to a file on disk so that restoreAgentExtensions can read them back.
func TestSaveWritesExtensionList(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	packageName := "datadog-agent"
	ctx := context.Background()

	// Setup: create DB with a package that has extensions
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)

	pkg := dbPackage{
		Name:       packageName,
		Version:    "7.50.0",
		Extensions: map[string]struct{}{"ddot": {}, "python": {}},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	// Save extensions to disk
	saveDir := t.TempDir()
	err = Save(ctx, packageName, saveDir)
	require.NoError(t, err)

	// Verify save file exists and contains extension names
	savePath := filepath.Join(saveDir, ".datadog-agent-extensions.txt")
	content, err := os.ReadFile(savePath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 2, "should have 2 extensions saved")
	assert.Contains(t, lines, "ddot")
	assert.Contains(t, lines, "python")
}

// TestSaveNoExtensionsNoFile verifies that Save does not create a file
// when the package has no extensions installed.
func TestSaveNoExtensionsNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	packageName := "datadog-agent"
	ctx := context.Background()

	// Setup: create DB with a package that has no extensions
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)

	pkg := dbPackage{
		Name:       packageName,
		Version:    "7.50.0",
		Extensions: map[string]struct{}{},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	// Save should succeed without creating a file
	saveDir := t.TempDir()
	err = Save(ctx, packageName, saveDir)
	require.NoError(t, err)

	// Verify no save file was created
	savePath := filepath.Join(saveDir, ".datadog-agent-extensions.txt")
	_, err = os.Stat(savePath)
	assert.True(t, os.IsNotExist(err), "save file should not exist when no extensions are installed")
}

// TestSavePackageNotFound verifies that Save returns an error
// when the package is not found in the database.
func TestSavePackageNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	ctx := context.Background()

	// Don't set up any packages -- the DB will be empty

	saveDir := t.TempDir()
	err := Save(ctx, "nonexistent-package", saveDir)
	require.Error(t, err, "Save should fail for a package not in the DB")
	assert.Contains(t, err.Error(), "not installed")
}
