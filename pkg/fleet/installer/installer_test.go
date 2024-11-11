// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

var testCtx = context.TODO()

type testPackageManager struct {
	installerImpl
}

func newTestPackageManager(t *testing.T, s *fixtures.Server, rootPath string, locksPath string) *testPackageManager {
	packages := repository.NewRepositories(rootPath, locksPath)
	db, err := db.New(filepath.Join(rootPath, "packages.db"))
	assert.NoError(t, err)
	return &testPackageManager{
		installerImpl{
			env:            &env.Env{},
			db:             db,
			downloader:     oci.NewDownloader(&env.Env{}, s.Client()),
			packages:       packages,
			userConfigsDir: t.TempDir(),
			packagesDir:    rootPath,
		},
	}
}

func (i *testPackageManager) ConfigFS(f fixtures.Fixture) fs.FS {
	return os.DirFS(filepath.Join(i.userConfigsDir, f.Package))
}

func TestInstallStable(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV1), installer.ConfigFS(fixtures.FixtureSimpleV1))
}

func TestInstallExperiment(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Experiment)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.ExperimentFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestInstallPromoteExperiment(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.PromoteExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestUninstallExperiment(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.RemoveExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	// we do not rollback configuration examples to their previous versions currently
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestInstallSkippedWhenAlreadyInstalled(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	lastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)

	err = installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r = installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	newLastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)
	assert.Equal(t, lastModTime, newLastModTime)
}

func TestReinstallAfterDBClean(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())
	defer installer.db.Close()

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	lastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)

	installer.db.DeletePackage(fixtures.FixtureSimpleV1.Package)

	err = installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r = installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	newLastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)
	assert.NotEqual(t, lastModTime, newLastModTime)
}

func latestModTimeFS(fsys fs.FS, dirPath string) (time.Time, error) {
	var latestTime time.Time

	// Read the directory entries
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return latestTime, err
	}

	for _, entry := range entries {
		// Get full path of the entry
		entryPath := path.Join(dirPath, entry.Name())

		// Get file info to access modification time
		info, err := fs.Stat(fsys, entryPath)
		if err != nil {
			return latestTime, err
		}

		// Update the latest modification time
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
		}

		// If the entry is a directory, recurse into it
		if entry.IsDir() {
			subLatestTime, err := latestModTimeFS(fsys, entryPath) // Recurse into subdirectory
			if err != nil {
				return latestTime, err
			}
			// Compare times
			if subLatestTime.After(latestTime) {
				latestTime = subLatestTime
			}
		}
	}

	return latestTime, nil
}

func TestPurge(t *testing.T) {
	s := fixtures.NewServer(t)
	rootPath := t.TempDir()
	installer := newTestPackageManager(t, s, rootPath, t.TempDir())

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)

	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)

	installer.Purge(testCtx)
	assert.NoFileExists(t, filepath.Join(rootPath, "packages.db"), "purge should remove the packages database")
	assert.NoDirExists(t, rootPath, "purge should remove the packages directory")
	assert.Nil(t, installer.db, "purge should close the packages database")
	assert.Nil(t, installer.cdn, "purge should close the CDN client")
}
