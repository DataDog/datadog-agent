// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
)

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

func TestInstallStable(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.s.Close()
	r := repository.Repository{
		RootPath:  t.TempDir(),
		LocksPath: t.TempDir(),
	}
	installer := newInstaller(&r)

	err := installer.installStable(fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
}

func TestInstallExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.s.Close()
	r := repository.Repository{
		RootPath:  t.TempDir(),
		LocksPath: t.TempDir(),
	}
	installer := newInstaller(&r)

	err := installer.installStable(fixtureSimpleV1.version, s.Image(fixtureSimpleV1))
	assert.NoError(t, err)
	err = installer.installExperiment(fixtureSimpleV2.version, s.Image(fixtureSimpleV2))
	assert.NoError(t, err)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.Equal(t, fixtureSimpleV2.version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.ExperimentFS())
}
