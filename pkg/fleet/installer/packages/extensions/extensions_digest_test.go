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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

// The extension embedded in FixtureSimpleV1WithExtension (see fixtures/server.go).
const fixtureExtensionName = "simple-extension"

// legacyDBPackage is the pre-digest-tracking schema where extension presence was
// stored as map[string]struct{} (no digest). Used only to seed test DBs.
type legacyDBPackage struct {
	Name       string              `json:"pkg"`
	Version    string              `json:"version"`
	Extensions map[string]struct{} `json:"extensions"`
}

// countingHooks counts PreInstallExtension invocations and can inject an error.
type countingHooks struct {
	preInstallCount int
	preInstallErr   error
}

func (c *countingHooks) PreInstallExtension(_ context.Context, _ string, _ string) error {
	c.preInstallCount++
	return c.preInstallErr
}

func (c *countingHooks) PostInstallExtension(_ context.Context, _ string, _ string, _ bool) error {
	return nil
}

func (c *countingHooks) PreRemoveExtension(_ context.Context, _ string, _ string) error {
	return nil
}

// realDigestForFixture downloads FixtureSimpleV1WithExtension and returns its OCI image digest.
func realDigestForFixture(t *testing.T, s *fixtures.Server) string {
	t.Helper()
	d := oci.NewDownloader(&env.Env{}, http.DefaultClient)
	pkg, err := d.Download(context.Background(), s.PackageURL(fixtures.FixtureSimpleV1WithExtension))
	require.NoError(t, err)
	digest, err := pkg.Image.Digest()
	require.NoError(t, err)
	return digest.String()
}

// seedExtensionDB writes a "simple/v1" entry with a single extension at the given digest.
func seedExtensionDB(t *testing.T, dir, extensionDigest string) {
	t.Helper()
	db, err := newExtensionsDB(filepath.Join(dir, "extensions.db"))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SetPackage(dbPackage{
		Name:       "simple",
		Version:    "v1",
		Extensions: map[string]string{fixtureExtensionName: extensionDigest},
	}, false))
}

// TestSkipExtensionInstallWhenDigestMatches verifies that Install does not call
// installSingle when the stored digest already matches the current OCI image digest.
func TestSkipExtensionInstallWhenDigestMatches(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	s := fixtures.NewServer(t)
	realDigest := realDigestForFixture(t, s)
	seedExtensionDB(t, tmpDir, realDigest)

	hooks := &countingHooks{}
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
	assert.Equal(t, 0, hooks.preInstallCount, "PreInstallExtension must not be called when digest matches")
}

// TestReinstallExtensionWhenDigestChanges verifies that Install triggers
// reinstallation when the stored digest differs from the current image digest.
func TestReinstallExtensionWhenDigestChanges(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	s := fixtures.NewServer(t)
	seedExtensionDB(t, tmpDir, "sha256:outdated")

	sentinel := errors.New("reinstall-triggered")
	hooks := &countingHooks{preInstallErr: sentinel}
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
	assert.Contains(t, err.Error(), "reinstall-triggered")
	assert.Equal(t, 1, hooks.preInstallCount, "PreInstallExtension must be called when digest differs")
}

// seedExtensionDBLegacy writes a "simple/v1" entry using the pre-digest schema
// (map[string]struct{}), exactly as old installer versions stored it.
func seedExtensionDBLegacy(t *testing.T, dir string) {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(dir, "extensions.db"), 0644, &bbolt.Options{
		FreelistType: bbolt.FreelistArrayType,
	})
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketExtensions)
		if err != nil {
			return err
		}
		data, err := json.Marshal(legacyDBPackage{
			Name:       "simple",
			Version:    "v1",
			Extensions: map[string]struct{}{fixtureExtensionName: {}},
		})
		if err != nil {
			return err
		}
		return b.Put(getKey("simple", false), data)
	}))
}

// TestReinstallExtensionWithLegacyFormat verifies that an extension stored in
// the old map[string]struct{} schema is always reinstalled after the digest upgrade.
// The old JSON encodes each extension value as {}, which triggers a migration path in
// GetPackage that preserves Name/Version and returns empty digest strings, causing
// Install to treat every extension as needing reinstallation.
func TestReinstallExtensionWithLegacyFormat(t *testing.T) {
	tmpDir := t.TempDir()
	ExtensionsDBDir = tmpDir

	s := fixtures.NewServer(t)
	seedExtensionDBLegacy(t, tmpDir)

	sentinel := errors.New("reinstall-triggered")
	hooks := &countingHooks{preInstallErr: sentinel}
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
	assert.Contains(t, err.Error(), "reinstall-triggered")
	assert.Equal(t, 1, hooks.preInstallCount, "PreInstallExtension must be called for legacy format")
}
