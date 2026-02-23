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

func (m *mockHooks) PostInstallExtension(_ context.Context, _ string, _ string, _ bool) error {
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

// TestSaveWritesExtensionList verifies that Save() writes the extension list to a file.
func TestSaveWritesExtensionList(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	// Setup DB with extensions
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()

	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0",
		Extensions: map[string]struct{}{"ddot": {}, "python": {}},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	// Save and verify
	saveDir := t.TempDir()
	err = Save(context.Background(), "datadog-agent", saveDir, false)
	require.NoError(t, err)

	// Verify file content
	savePath := filepath.Join(saveDir, ".datadog-agent-extensions.txt")
	content, err := os.ReadFile(savePath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.Len(t, lines, 2)
	assert.Contains(t, lines, "ddot")
	assert.Contains(t, lines, "python")
}

// TestSaveNoExtensionsNoFile verifies that no file is created when no extensions exist.
func TestSaveNoExtensionsNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	// Setup DB with no extensions
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()

	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0",
		Extensions: map[string]struct{}{},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	// Save and verify no file created
	saveDir := t.TempDir()
	err = Save(context.Background(), "datadog-agent", saveDir, false)
	require.NoError(t, err)

	// Verify file does not exist
	savePath := filepath.Join(saveDir, ".datadog-agent-extensions.txt")
	_, err = os.Stat(savePath)
	assert.True(t, os.IsNotExist(err), "file should not exist when there are no extensions")
}

// TestSaveExperimentReadsExperimentEntry verifies that Save(..., isExperiment=true) reads
// from the experiment DB entry rather than the stable one.
func TestSaveExperimentReadsExperimentEntry(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)

	// Store stable with "python", experiment with "ddot"
	err = db.SetPackage(dbPackage{Name: "datadog-agent", Version: "7.50.0", Extensions: map[string]struct{}{"python": {}}}, false)
	require.NoError(t, err)
	err = db.SetPackage(dbPackage{Name: "datadog-agent", Version: "7.51.0", Extensions: map[string]struct{}{"ddot": {}}}, true)
	require.NoError(t, err)
	db.Close()

	saveDir := t.TempDir()
	err = Save(context.Background(), "datadog-agent", saveDir, true)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(saveDir, ".datadog-agent-extensions.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ddot", strings.TrimSpace(string(content)), "should save experiment extension, not stable")
}

// TestSavePackageNotFound verifies that an error is returned when package is not in DB.
func TestSavePackageNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	// Setup empty DB
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	db.Close()

	// Try to save non-existent package
	saveDir := t.TempDir()
	err = Save(context.Background(), "datadog-agent", saveDir, false)

	require.Error(t, err, "should error when package not found")
	assert.ErrorIs(t, err, errPackageNotFound, "error should indicate package not found")
}
