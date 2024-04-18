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

	"github.com/DataDog/datadog-agent/pkg/installer/repository"
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

func newTestPackageManager(t *testing.T) *testPackageManager {
	repositories := repository.NewRepositories(t.TempDir(), t.TempDir())
	return &testPackageManager{
		packageManager{
			repositories: repositories,
			configsDir:   t.TempDir(),
		},
	}
}

func (i *testPackageManager) ConfigFS(f fixture) fs.FS {
	return os.DirFS(filepath.Join(i.configsDir, f.pkg))
}

func TestInstallStable(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	installer := newTestPackageManager(t)

	err := installer.installStable(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.ConfigFS(fixtureSimpleV1), installer.ConfigFS(fixtureSimpleV1))
}

func TestInstallExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	installer := newTestPackageManager(t)

	err := installer.installStable(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV2.version, s.Image(fixtureSimpleV2))
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.Equal(t, fixtureSimpleV2.version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.ExperimentFS())
	assertEqualFS(t, s.ConfigFS(fixtureSimpleV2), installer.ConfigFS(fixtureSimpleV2))
}

func TestInstallPromoteExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	installer := newTestPackageManager(t)

	err := installer.installStable(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV2.version, s.Image(fixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.promoteExperiment(testCtx, fixtureSimpleV1.pkg)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV2.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.StableFS())
	assertEqualFS(t, s.ConfigFS(fixtureSimpleV2), installer.ConfigFS(fixtureSimpleV2))
}

func TestUninstallExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	installer := newTestPackageManager(t)

	err := installer.installStable(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(testCtx, fixtureSimpleV1.pkg, fixtureSimpleV2.version, s.Image(fixtureSimpleV2))
	assert.NoError(t, err)
	err = installer.uninstallExperiment(testCtx, fixtureSimpleV1.pkg)
	assert.NoError(t, err)
	r := installer.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
	// we do not rollback configuration examples to their previous versions currently
	assertEqualFS(t, s.ConfigFS(fixtureSimpleV2), installer.ConfigFS(fixtureSimpleV2))
}
