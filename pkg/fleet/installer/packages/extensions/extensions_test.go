// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// mockHooks implements ExtensionHooks interface for testing
type mockHooks struct {
	preInstallErr  error
	postInstallErr error
	preRemoveErr   error

	// preInstallCalls counts PreInstallExtension invocations. preInstallFailFrom,
	// when >0, makes PreInstallExtension return an error from that (1-indexed) call
	// onward — used to fail the rollback pre-install while letting the forward one
	// succeed.
	preInstallCalls    int
	preInstallFailFrom int
}

func (m *mockHooks) PreInstallExtension(_ context.Context, _ string, _ string) error {
	m.preInstallCalls++
	if m.preInstallFailFrom > 0 && m.preInstallCalls >= m.preInstallFailFrom {
		return errors.New("pre-install-failed")
	}
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
		Extensions: map[string]string{"python": "sha256:stable"},
	}
	err = db.SetPackage(stablePkg, false) // isExperiment=false
	require.NoError(t, err)

	// Store experiment package with extension "ruby" (same package name)
	expPkg := dbPackage{
		Name:       packageName,
		Version:    "7.51.0",
		Extensions: map[string]string{"ruby": "sha256:experiment"},
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

// TestSetPackageVersionIdempotent verifies that calling SetPackageVersion with the same
// pkg, version, and isExperiment does not wipe the extensions list.
func TestSetPackageVersionIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()

	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0",
		Extensions: map[string]string{"python": "sha256:abc", "ruby": "sha256:def"},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)

	err = db.SetPackageVersion("datadog-agent", "7.50.0", false)
	require.NoError(t, err)

	got, err := db.GetPackage("datadog-agent", false)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"python": "sha256:abc", "ruby": "sha256:def"}, got.Extensions, "extensions should be preserved after idempotent SetPackageVersion")
}

// TestSetPackageVersionIdempotentWithLegacyFormat verifies that SetPackageVersion is a no-op
// when the stored entry uses the old map[string]struct{} schema and the version matches,
// preserving the legacy data so GetPackage can later migrate it to trigger reinstalls.
func TestSetPackageVersionIdempotentWithLegacyFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a legacy-format entry directly via bbolt.
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		type legacyPkg struct {
			Name       string              `json:"pkg"`
			Version    string              `json:"version"`
			Extensions map[string]struct{} `json:"extensions"`
		}
		data, err := json.Marshal(legacyPkg{Name: "datadog-agent", Version: "7.50.0", Extensions: map[string]struct{}{"python": {}}})
		if err != nil {
			return err
		}
		return b.Put(getKey("datadog-agent", false), data)
	}))

	err = db.SetPackageVersion("datadog-agent", "7.50.0", false)
	require.NoError(t, err)

	// The legacy entry must still be readable via GetPackage (not wiped to empty).
	got, err := db.GetPackage("datadog-agent", false)
	require.NoError(t, err)
	assert.Equal(t, "7.50.0", got.Version)
	assert.Contains(t, got.Extensions, "python", "legacy extension key must survive SetPackageVersion no-op")
	db.Close()
}

// TestSetPackageVersionWipesExtensionsOnVersionChange verifies that calling SetPackageVersion
// with a new version resets the extensions list (intentional behavior).
func TestSetPackageVersionWipesExtensionsOnVersionChange(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()

	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0",
		Extensions: map[string]string{"python": "sha256:abc"},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)

	err = db.SetPackageVersion("datadog-agent", "7.51.0", false)
	require.NoError(t, err)

	got, err := db.GetPackage("datadog-agent", false)
	require.NoError(t, err)
	assert.Equal(t, "7.51.0", got.Version)
	assert.Empty(t, got.Extensions, "extensions should be wiped on version change")
}

// TestInstallGroupsByRegistry verifies that Install correctly groups extensions
// by registry, downloading from overridden registries where configured.
func TestInstallGroupsByRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	ctx := context.Background()
	hooks := &mockHooks{}

	// Seed the DB with the package
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0-1",
		Extensions: map[string]string{},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	downloader := oci.NewDownloader(&env.Env{}, http.DefaultClient)
	overrides := map[string]ExtensionRegistry{
		"ddot": {
			URL:      "custom.registry.com",
			Auth:     "password",
			Username: "user",
			Password: "pass",
		},
	}

	// Install will fail due to no real registry, but we can verify the function
	// accepts overrides without panic and attempts to download.
	err = Install(ctx, downloader, "oci://install.datadoghq.com/agent-package:7.50.0-1",
		[]string{"ddot", "other-ext"}, false, hooks, overrides)

	// We expect download errors (no real registry), not a panic or grouping error.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not download package")
}

// TestInstallNilOverridesBackwardsCompat verifies that nil overrides work (backwards compat).
func TestInstallNilOverridesBackwardsCompat(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	ctx := context.Background()
	hooks := &mockHooks{}

	// Seed the DB with the package
	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	pkg := dbPackage{
		Name:       "datadog-agent",
		Version:    "7.50.0-1",
		Extensions: map[string]string{},
	}
	err = db.SetPackage(pkg, false)
	require.NoError(t, err)
	db.Close()

	downloader := oci.NewDownloader(&env.Env{}, http.DefaultClient)

	// nil overrides should work exactly as before
	err = Install(ctx, downloader, "oci://install.datadoghq.com/agent-package:7.50.0-1",
		[]string{"ddot"}, false, hooks, nil)

	// Expect download failure (no real registry), not a grouping error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not download package")
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

// setupReplaceTest creates a temp environment for testing the replace path in installSingle:
//   - Sets ExtensionsPackagesPath and ExtensionsDBDir to isolated temp dirs
//   - Creates an existing extension directory with a sentinel file ("old-sentinel")
//   - Seeds the DB with an outdated digest so Install triggers replace=true
//
// Returns the absolute path to the extension directory.
func setupReplaceTest(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	ExtensionsPackagesPath = tmpDir
	ExtensionsDBDir = tmpDir
	t.Cleanup(func() {
		ExtensionsPackagesPath = paths.PackagesPath
		ExtensionsDBDir = paths.RunPath
	})

	extPath := filepath.Join(tmpDir, "simple", "v1", "ext", fixtureExtensionName)
	require.NoError(t, os.MkdirAll(extPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(extPath, "old-sentinel"), []byte("old"), 0644))

	db, err := newExtensionsDB(filepath.Join(tmpDir, "extensions.db"))
	require.NoError(t, err)
	require.NoError(t, db.SetPackage(dbPackage{
		Name:       "simple",
		Version:    "v1",
		Extensions: map[string]string{fixtureExtensionName: "sha256:outdated"},
	}, false))
	db.Close()

	return extPath
}

// TestInstallSingleReplaceSucceeds verifies that when a previous extension exists
// and the digest has changed, Install replaces it with new content and leaves no
// backup directories behind.
func TestInstallSingleReplaceSucceeds(t *testing.T) {
	extPath := setupReplaceTest(t)
	s := fixtures.NewServer(t)
	hooks := &mockHooks{}

	err := Install(
		context.Background(),
		oci.NewDownloader(&env.Env{}, http.DefaultClient),
		s.PackageURL(fixtures.FixtureSimpleV1WithExtension),
		[]string{fixtureExtensionName},
		false,
		hooks,
		nil,
	)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(extPath, "old-sentinel"))
	assert.True(t, os.IsNotExist(statErr), "old sentinel file should be removed after successful replace")

	_, statErr = os.Stat(extPath)
	assert.NoError(t, statErr, "extension directory should exist after successful replace")

	entries, err := os.ReadDir(filepath.Dir(extPath))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "-backup-", "no backup directories should remain after successful replace")
	}
}

// TestInstallSingleRollsBackOnPostInstallFailure verifies that when PostInstallExtension
// fails after the new content is already in place, the old extension is restored from backup.
func TestInstallSingleRollsBackOnPostInstallFailure(t *testing.T) {
	extPath := setupReplaceTest(t)
	s := fixtures.NewServer(t)
	hooks := &mockHooks{postInstallErr: errors.New("post-install-failed")}

	err := Install(
		context.Background(),
		oci.NewDownloader(&env.Env{}, http.DefaultClient),
		s.PackageURL(fixtures.FixtureSimpleV1WithExtension),
		[]string{fixtureExtensionName},
		false,
		hooks,
		nil,
	)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(extPath, "old-sentinel"))
	assert.NoError(t, statErr, "old sentinel file should be restored after post-install failure rollback")
}

// TestInstallSingleRestoresBackupWhenRollbackPreInstallFails verifies that a
// failing pre-install hook during rollback does not prevent the previous
// extension from being restored. The previous installation was already removed
// on the replace path, so skipping the restore would leave no extension at all.
func TestInstallSingleRestoresBackupWhenRollbackPreInstallFails(t *testing.T) {
	extPath := setupReplaceTest(t)
	s := fixtures.NewServer(t)
	// postInstallErr triggers rollback; preInstallFailFrom=2 lets the forward
	// pre-install (call 1) succeed but fails the rollback pre-install (call 2).
	hooks := &mockHooks{postInstallErr: errors.New("post-install-failed"), preInstallFailFrom: 2}

	err := Install(
		context.Background(),
		oci.NewDownloader(&env.Env{}, http.DefaultClient),
		s.PackageURL(fixtures.FixtureSimpleV1WithExtension),
		[]string{fixtureExtensionName},
		false,
		hooks,
		nil,
	)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(extPath, "old-sentinel"))
	assert.NoError(t, statErr, "old sentinel file should be restored even when the rollback pre-install hook fails")
}

// TestInstallSinglePreRemoveFailurePreservesExtension verifies that when the pre-remove
// hook fails, the original extension is left untouched (removeSingle bails before
// os.RemoveAll, so the extension is never removed from disk).
func TestInstallSinglePreRemoveFailurePreservesExtension(t *testing.T) {
	extPath := setupReplaceTest(t)
	s := fixtures.NewServer(t)
	hooks := &mockHooks{preRemoveErr: errors.New("pre-remove-failed")}

	err := Install(
		context.Background(),
		oci.NewDownloader(&env.Env{}, http.DefaultClient),
		s.PackageURL(fixtures.FixtureSimpleV1WithExtension),
		[]string{fixtureExtensionName},
		false,
		hooks,
		nil,
	)
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(extPath, "old-sentinel"))
	assert.NoError(t, statErr, "original extension should be intact when pre-remove hook fails")
}

// TestRemoveAllMissingDB verifies that RemoveAll is a no-op when the extensions
// database file does not exist. This guards against the prerm hook failing on
// hosts where extensions were never initialized.
func TestRemoveAllMissingDB(t *testing.T) {
	ExtensionsDBDir = filepath.Join(t.TempDir(), "does-not-exist")

	err := RemoveAll(context.Background(), "datadog-agent", false, &mockHooks{})
	require.NoError(t, err, "RemoveAll should return nil when the extensions DB does not exist")
}
