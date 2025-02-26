// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package repository

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestRepository(t *testing.T, dir string, stablePackageName string, preRemoveHook PreRemoveHook) *Repository {
	repositoryPath := path.Join(dir, "repository")
	os.MkdirAll(repositoryPath, 0755)
	stablePackagePath := createTestDownloadedPackage(t, dir, stablePackageName)
	r := Repository{
		rootPath: repositoryPath,
	}
	if preRemoveHook != nil {
		r.preRemoveHooks = map[string]PreRemoveHook{"repository": preRemoveHook}
	}
	err := r.Create(context.Background(), stablePackageName, stablePackagePath)
	assert.NoError(t, err)
	return &r
}

func createTestDownloadedPackage(t *testing.T, dir string, packageName string) string {
	downloadPath := path.Join(dir, "download", packageName)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)
	return downloadPath
}

func assertLinkTarget(t *testing.T, repository *Repository, link string, target string) {
	linkPath := path.Join(repository.rootPath, link)
	assert.FileExists(t, linkPath)
	linkTarget, err := linkRead(linkPath)
	assert.NoError(t, err)
	assert.Equal(t, target, filepath.Base(linkTarget))
}

func TestCreateFresh(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)
	state, err := repository.GetState()

	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.NoError(t, err)
	assert.True(t, state.HasStable())
	assert.Equal(t, "v1", state.Stable)
	assert.False(t, state.HasExperiment())
	assertLinkTarget(t, repository, stableVersionLink, "v1")
	assertLinkTarget(t, repository, experimentVersionLink, "v1")
}

func TestCreateOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old", nil)

	repository := createTestRepository(t, dir, "v1", nil)

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.NoDirExists(t, path.Join(oldRepository.rootPath, "old"))
}

func TestCreateOverwriteWithHookAllow(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old", nil)

	hook := func(context.Context, string) (bool, error) { return true, nil }
	repository := createTestRepository(t, dir, "v1", hook)

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.NoDirExists(t, path.Join(repository.rootPath, "old"))
}

func TestCreateOverwriteWithHookDeny(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old", nil)

	hook := func(context.Context, string) (bool, error) { return false, nil }
	repository := createTestRepository(t, dir, "v1", hook)

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.DirExists(t, path.Join(repository.rootPath, "old"))
}

func TestSetExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	state, err := repository.GetState()

	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
	assert.True(t, state.HasStable())
	assert.Equal(t, "v1", state.Stable)
	assert.True(t, state.HasExperiment())
	assert.Equal(t, "v2", state.Experiment)
	assertLinkTarget(t, repository, stableVersionLink, "v1")
	assertLinkTarget(t, repository, experimentVersionLink, "v2")
}

func TestSetExperimentTwice(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)
	experiment1DownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")
	experiment2DownloadPackagePath := createTestDownloadedPackage(t, dir, "v3")

	err := repository.SetExperiment(context.Background(), "v2", experiment1DownloadPackagePath)
	assert.NoError(t, err)
	err = repository.SetExperiment(context.Background(), "v3", experiment2DownloadPackagePath)
	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestSetExperimentBeforeStable(t *testing.T) {
	dir := t.TempDir()
	repository := Repository{
		rootPath: dir,
	}
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.Error(t, err)
}

func TestPromoteExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.PromoteExperiment(context.Background())
	assert.NoError(t, err)
	state, err := repository.GetState()
	assert.NoError(t, err)

	assert.NoDirExists(t, path.Join(repository.rootPath, "v1"))
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
	assert.True(t, state.HasStable())
	assert.Equal(t, "v2", state.Stable)
	assert.False(t, state.HasExperiment())
	assertLinkTarget(t, repository, stableVersionLink, "v2")
	assertLinkTarget(t, repository, experimentVersionLink, "v2")
}

func TestPromoteExperimentWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)

	err := repository.PromoteExperiment(context.Background())
	assert.Error(t, err)
}

func TestDeleteExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.DeleteExperiment(context.Background())
	assert.NoError(t, err)
	assert.NoDirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestDeleteExperimentWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)

	err := repository.DeleteExperiment(context.Background())
	assert.NoError(t, err)
}

func TestDeleteExperimentWithHookAllow(t *testing.T) {
	dir := t.TempDir()
	hook := func(context.Context, string) (bool, error) { return true, nil }
	repository := createTestRepository(t, dir, "v1", hook)
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.DeleteExperiment(context.Background())
	assert.NoError(t, err)
	assert.NoDirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestDeleteExperimentWithHookDeny(t *testing.T) {
	dir := t.TempDir()
	hook := func(context.Context, string) (bool, error) { return false, nil }
	repository := createTestRepository(t, dir, "v1", hook)
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment(context.Background(), "v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.DeleteExperiment(context.Background())
	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestMigrateRepositoryWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)

	err := os.Remove(path.Join(repository.rootPath, experimentVersionLink))
	assert.NoError(t, err)
	assert.NoFileExists(t, path.Join(repository.rootPath, experimentVersionLink))
	err = repository.Cleanup(context.Background())
	assert.NoError(t, err)
	assertLinkTarget(t, repository, stableVersionLink, "v1")
	assertLinkTarget(t, repository, experimentVersionLink, "v1")
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)

	err := repository.Delete(context.Background())
	assert.NoError(t, err)
	assert.NoDirExists(t, repository.rootPath)
}

func TestDeleteHookAllow(t *testing.T) {
	dir := t.TempDir()
	hook := func(context.Context, string) (bool, error) { return true, nil }
	repository := createTestRepository(t, dir, "v1", hook)

	err := repository.Delete(context.Background())
	assert.NoError(t, err)
	assert.NoDirExists(t, repository.rootPath)
}

func TestDeleteHookDeny(t *testing.T) {
	dir := t.TempDir()
	hook := func(context.Context, string) (bool, error) { return false, nil }
	repository := createTestRepository(t, dir, "v1", hook)

	err := repository.Delete(context.Background())
	assert.Error(t, err)
	assert.DirExists(t, repository.rootPath)
}

func TestDeleteExtraFilesDoNotPreventDeletion(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1", nil)

	extraFilePath := path.Join(repository.rootPath, "extra")
	err := os.WriteFile(extraFilePath, []byte("extra"), 0644)
	assert.NoError(t, err)

	err = repository.Delete(context.Background())
	assert.NoError(t, err)
	assert.NoDirExists(t, repository.rootPath)
}

func TestDeleteHookDenyDoesNotPreventReinstall(t *testing.T) {
	dir := t.TempDir()
	hook := func(context.Context, string) (bool, error) { return false, nil }
	oldRepository := createTestRepository(t, dir, "old", hook)

	err := oldRepository.Delete(context.Background())
	assert.Error(t, err)

	repository := createTestRepository(t, dir, "v1", nil)

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.NoDirExists(t, path.Join(oldRepository.rootPath, "old"))
}
