// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package installer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/installer/packages/fixtures"
	"github.com/DataDog/datadog-agent/pkg/installer/packages/repository"
)

var testCtx = context.TODO()

func assertEqualFS(t *testing.T, expected fs.FS, actual fs.FS) {
	t.Helper()
	err := fsContainsAll(expected, actual)
	assert.NoError(t, err)
	err = fsContainsAll(actual, expected)
	assert.NoError(t, err)
}

func fsContainsAll(a fs.FS, b fs.FS) error {
	return fs.WalkDir(a, ".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entryA, err := a.Open(path)
		if err != nil {
			return err
		}
		entryB, err := b.Open(path)
		if err != nil {
			return err
		}
		entryAStat, err := entryA.Stat()
		if err != nil {
			return err
		}
		entryBStat, err := entryB.Stat()
		if err != nil {
			return err
		}
		if entryAStat.IsDir() != entryBStat.IsDir() {
			return fmt.Errorf("files %s are not equal", path)
		}
		if entryAStat.IsDir() {
			return nil
		}
		contentA, err := io.ReadAll(entryA)
		if err != nil {
			return err
		}
		contentB, err := io.ReadAll(entryB)
		if err != nil {
			return err
		}
		if !bytes.Equal(contentA, contentB) {
			return fmt.Errorf("files %s do not have the same content: %s != %s", path, contentA, contentB)
		}
		return nil
	})
}

type testPackageManager struct {
	packageManager
}

func newTestPackageManager(t *testing.T, rootPath string, locksPath string) *testPackageManager {
	repositories := repository.NewRepositories(rootPath, locksPath)
	return &testPackageManager{
		packageManager{
			repositories: repositories,
			configsDir:   t.TempDir(),
			tmpDirPath:   rootPath,
		},
	}
}

func (i *testPackageManager) ConfigFS(f fixtures.Fixture) fs.FS {
	return os.DirFS(filepath.Join(i.configsDir, f.Package))
}

func TestInstallStable(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	installer := newTestPackageManager(t, t.TempDir(), t.TempDir())

	err := installer.installStable(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV1.Version, s.Image(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV1), installer.ConfigFS(fixtures.FixtureSimpleV1))
}

func TestInstallExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	installer := newTestPackageManager(t, t.TempDir(), t.TempDir())

	err := installer.installStable(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV1.Version, s.Image(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV2.Version, s.Image(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.ExperimentFS())
	assertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestInstallPromoteExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	installer := newTestPackageManager(t, t.TempDir(), t.TempDir())

	err := installer.installStable(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV1.Version, s.Image(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV2.Version, s.Image(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.promoteExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
	assertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}

func TestUninstallExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	installer := newTestPackageManager(t, t.TempDir(), t.TempDir())

	err := installer.installStable(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV1.Version, s.Image(fixtures.FixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV2.Version, s.Image(fixtures.FixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.uninstallExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	// we do not rollback configuration examples to their previous versions currently
	assertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
}
