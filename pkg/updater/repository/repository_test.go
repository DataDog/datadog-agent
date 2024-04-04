// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/updater/service"
	"github.com/stretchr/testify/assert"
)

func createTestRepository(t *testing.T, dir string, stablePackageName string) *Repository {
	repositoryPath := path.Join(dir, "repository")
	assert.Nil(t, service.BuildHelperForTests(repositoryPath, t.TempDir(), true))
	locksPath := path.Join(dir, "run")
	os.MkdirAll(repositoryPath, 0755)
	os.MkdirAll(locksPath, 0777)
	stablePackagePath := createTestDownloadedPackage(t, dir, stablePackageName)
	r := Repository{
		rootPath:  repositoryPath,
		locksPath: locksPath,
	}
	err := r.Create(stablePackageName, stablePackagePath)
	assert.NoError(t, err)
	return &r
}

func createTestDownloadedPackage(t *testing.T, dir string, packageName string) string {
	downloadPath := path.Join(dir, "download", packageName)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)
	return downloadPath
}

func TestCreateFresh(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")

	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
}

func TestCreateOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old")

	repository := createTestRepository(t, dir, "v1")

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.NoDirExists(t, path.Join(oldRepository.rootPath, "old"))
}

func TestCreateOverwriteWithLockedPackage(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old")
	err := os.MkdirAll(path.Join(oldRepository.locksPath, "garbagetocollect"), 0777)
	assert.NoError(t, err)

	// Add a running process... our own! So we're sure it's running.
	err = os.MkdirAll(path.Join(oldRepository.locksPath, "old"), 0777)
	assert.NoError(t, err)
	err = os.WriteFile(
		path.Join(oldRepository.locksPath, "old", fmt.Sprint(os.Getpid())),
		nil,
		0644,
	)
	assert.NoError(t, err)

	repository := createTestRepository(t, dir, "v1")

	assert.Equal(t, oldRepository.rootPath, repository.rootPath)
	assert.DirExists(t, repository.rootPath)
	assert.DirExists(t, path.Join(repository.rootPath, "v1"))
	assert.DirExists(t, path.Join(repository.rootPath, "old"))
	assert.NoDirExists(t, path.Join(oldRepository.locksPath, "garbagetocollect"))
}

func TestSetExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestSetExperimentTwice(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experiment1DownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")
	experiment2DownloadPackagePath := createTestDownloadedPackage(t, dir, "v3")

	err := repository.SetExperiment("v2", experiment1DownloadPackagePath)
	assert.NoError(t, err)
	err = repository.SetExperiment("v3", experiment2DownloadPackagePath)
	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestSetExperimentBeforeStable(t *testing.T) {
	dir := t.TempDir()
	repository := Repository{
		rootPath: dir,
	}
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.Error(t, err)
}

func TestPromoteExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.PromoteExperiment()
	assert.NoError(t, err)
	assert.NoDirExists(t, path.Join(repository.rootPath, "v1"))
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestPromoteExperimentWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")

	err := repository.PromoteExperiment()
	assert.Error(t, err)
}

func TestDeleteExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	err = repository.DeleteExperiment()
	assert.NoError(t, err)
	assert.NoDirExists(t, path.Join(repository.rootPath, "v2"))
}

func TestDeleteExperimentWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")

	err := repository.DeleteExperiment()
	assert.NoError(t, err)
}

func TestDeleteExperimentWithLockedPackage(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.NoError(t, err)

	// Add a running process... our own! So we're sure it's running.
	err = os.MkdirAll(path.Join(repository.locksPath, "v2"), 0766)
	assert.NoError(t, err)
	err = os.WriteFile(
		path.Join(repository.locksPath, "v2", fmt.Sprint(os.Getpid())),
		nil,
		0644,
	)
	assert.NoError(t, err)

	// Add a running process that's not running to check its deletion
	err = os.MkdirAll(path.Join(repository.locksPath, "v2"), 0766)
	assert.NoError(t, err)
	err = os.WriteFile(
		path.Join(repository.locksPath, "v2", "-1"), // We're sure not to hit a running process
		nil,
		0644,
	)
	assert.NoError(t, err)

	err = repository.DeleteExperiment()
	assert.NoError(t, err)
	assert.DirExists(t, path.Join(repository.rootPath, "v2"))
	assert.DirExists(t, path.Join(repository.locksPath, "v2"))
	assert.NoFileExists(t, path.Join(repository.locksPath, "v2", "-1"))
	assert.FileExists(t, path.Join(repository.locksPath, "v2", fmt.Sprint(os.Getpid())))
}
