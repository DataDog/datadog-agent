// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeVersion creates a version directory directly in the repository, which is what the macOS
// system installer does.
func writeVersion(t *testing.T, rootPath string, version string) string {
	t.Helper()
	path := filepath.Join(rootPath, version)
	require.NoError(t, os.MkdirAll(filepath.Join(path, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "bin", "agent"), []byte("binary"), 0755))
	return path
}

func stableRepository(t *testing.T) (*Repository, string) {
	t.Helper()
	// Named "repository" rather than "datadog-agent" so cleanup's Linux-only special case for
	// the agent package stays out of the way; adoption has nothing to do with it.
	rootPath := filepath.Join(t.TempDir(), "repository")
	require.NoError(t, os.MkdirAll(rootPath, 0755))
	repository := &Repository{rootPath: rootPath}
	source := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "marker"), []byte("stable"), 0644))
	require.NoError(t, repository.Create(context.Background(), "1.0.0", source))
	return repository, rootPath
}

// TestAdoptExperimentNamesAPayloadThatIsAlreadyInPlace is the case SetExperiment cannot serve: the
// payload was written into the repository by something other than the installer.
func TestAdoptExperimentNamesAPayloadThatIsAlreadyInPlace(t *testing.T) {
	repository, rootPath := stableRepository(t)
	writeVersion(t, rootPath, "2.0.0")

	require.NoError(t, repository.AdoptExperiment(context.Background(), "2.0.0"))

	state, err := repository.GetState()
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", state.Stable)
	assert.Equal(t, "2.0.0", state.Experiment)
	// And the payload survived the cleanup that adoption ends with.
	_, err = os.Stat(filepath.Join(rootPath, "2.0.0", "bin", "agent"))
	assert.NoError(t, err)
}

// TestSetExperimentRefusesAPayloadThatIsAlreadyInPlace records why AdoptExperiment exists at all.
// If this ever starts passing, adoption can go away.
func TestSetExperimentRefusesAPayloadThatIsAlreadyInPlace(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("windows repairs an existing directory instead of refusing it")
	}
	repository, rootPath := stableRepository(t)
	path := writeVersion(t, rootPath, "2.0.0")

	err := repository.SetExperiment(context.Background(), "2.0.0", path)
	assert.Error(t, err)
}

// TestAdoptExperimentReclaimsAPreviousExperiment is the other half of the ordering: adoption still
// has to clean up, or a host that experiments repeatedly accumulates every version it ever tried.
func TestAdoptExperimentReclaimsAPreviousExperiment(t *testing.T) {
	repository, rootPath := stableRepository(t)
	writeVersion(t, rootPath, "2.0.0")
	require.NoError(t, repository.AdoptExperiment(context.Background(), "2.0.0"))
	writeVersion(t, rootPath, "3.0.0")
	require.NoError(t, repository.AdoptExperiment(context.Background(), "3.0.0"))

	_, err := os.Stat(filepath.Join(rootPath, "2.0.0"))
	assert.True(t, os.IsNotExist(err), "the previous experiment was not reclaimed")
	_, err = os.Stat(filepath.Join(rootPath, "1.0.0"))
	assert.NoError(t, err, "stable was reclaimed")
}

// TestAdoptExperimentRefusesWhatIsNotThere keeps a link from ever naming a version that does not
// exist, which would leave the -exp jobs pointing into nothing.
func TestAdoptExperimentRefusesWhatIsNotThere(t *testing.T) {
	repository, rootPath := stableRepository(t)

	assert.Error(t, repository.AdoptExperiment(context.Background(), "2.0.0"))

	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "3.0.0"), []byte("not a directory"), 0644))
	assert.Error(t, repository.AdoptExperiment(context.Background(), "3.0.0"))
}

// TestAdoptExperimentRefusesStableAndTheLinkNames guards the degenerate names. Adopting stable as
// the experiment would make promote a no-op and hide a failed update as a success.
func TestAdoptExperimentRefusesStableAndTheLinkNames(t *testing.T) {
	repository, _ := stableRepository(t)

	assert.Error(t, repository.AdoptExperiment(context.Background(), "1.0.0"))
	assert.Error(t, repository.AdoptExperiment(context.Background(), "stable"))
	assert.Error(t, repository.AdoptExperiment(context.Background(), "experiment"))
	assert.Error(t, repository.AdoptExperiment(context.Background(), ""))
}

// TestPromoteAndDeleteWorkOnAnAdoptedExperiment is the point of adopting rather than inventing a
// parallel set of operations: everything downstream is the code every platform already runs.
func TestPromoteAndDeleteWorkOnAnAdoptedExperiment(t *testing.T) {
	repository, rootPath := stableRepository(t)
	writeVersion(t, rootPath, "2.0.0")
	require.NoError(t, repository.AdoptExperiment(context.Background(), "2.0.0"))

	require.NoError(t, repository.PromoteExperiment(context.Background()))
	state, err := repository.GetState()
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", state.Stable)

	writeVersion(t, rootPath, "3.0.0")
	require.NoError(t, repository.AdoptExperiment(context.Background(), "3.0.0"))
	require.NoError(t, repository.DeleteExperiment(context.Background()))
	state, err = repository.GetState()
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", state.Stable)
	assert.Empty(t, state.Experiment)
}

// TestHasVersion covers the question the darwin install path asks before deciding whether to
// download and reinstall a version at all.
func TestHasVersion(t *testing.T) {
	repository, rootPath := stableRepository(t)

	has, err := repository.HasVersion("1.0.0")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = repository.HasVersion("2.0.0")
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, os.WriteFile(filepath.Join(rootPath, "3.0.0"), []byte("file"), 0644))
	has, err = repository.HasVersion("3.0.0")
	require.NoError(t, err)
	assert.False(t, has, "a file is not a version directory")

	_, err = repository.HasVersion("")
	assert.Error(t, err)
}
