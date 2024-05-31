// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package installer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
	repositories := repository.NewRepositories(rootPath, locksPath)
	db, err := db.New(filepath.Join(rootPath, "packages.db"))
	assert.NoError(t, err)
	return &testPackageManager{
		installerImpl{
			db:           db,
			downloader:   oci.NewDownloader(&env.Env{}, s.Client()),
			repositories: repositories,
			configsDir:   t.TempDir(),
			tmpDirPath:   rootPath,
			packagesDir:  rootPath,
		},
	}
}

func (i *testPackageManager) ConfigFS(f fixtures.Fixture) fs.FS {
	return os.DirFS(filepath.Join(i.configsDir, f.Package))
}

func TestInstallStable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: Failing test on macOS - #incident-26965")
	}

	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV1), installer.ConfigFS(fixtures.FixtureSimpleV1))
}

func TestInstallExperiment(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: Failing test on macOS - #incident-26965")
	}

	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Experiment)
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.ExperimentFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestInstallPromoteExperiment(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: Failing test on macOS - #incident-26965")
	}

	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.PromoteExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestUninstallExperiment(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: Failing test on macOS - #incident-26965")
	}

	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir(), t.TempDir())

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.RemoveExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	// we do not rollback configuration examples to their previous versions currently
	fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}
