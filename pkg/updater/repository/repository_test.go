// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestRepository(t *testing.T, dir string, stablePackageName string) Repository {
	repositoryPath := path.Join(dir, "repository")
	stablePackagePath := createTestDownloadedPackage(t, dir, stablePackageName)
	r := Repository{
		RootPath: repositoryPath,
	}
	err := r.Create(stablePackageName, stablePackagePath)
	assert.NoError(t, err)
	return r
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

	_, err := os.Stat(repository.RootPath)
	assert.NoError(t, err)
	_, err = os.Stat(path.Join(repository.RootPath, "v1"))
	assert.NoError(t, err)
}

func TestCreateOverwrite(t *testing.T) {
	dir := t.TempDir()
	oldRepository := createTestRepository(t, dir, "old")
	repository := createTestRepository(t, dir, "v1")

	assert.Equal(t, oldRepository.RootPath, repository.RootPath)
	_, err := os.Stat(repository.RootPath)
	assert.NoError(t, err)
	_, err = os.Stat(path.Join(repository.RootPath, "v1"))
	assert.NoError(t, err)
	_, err = os.Stat(path.Join(oldRepository.RootPath, "old"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSetExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")
	experimentDownloadPackagePath := createTestDownloadedPackage(t, dir, "v2")

	err := repository.SetExperiment("v2", experimentDownloadPackagePath)
	assert.NoError(t, err)
	_, err = os.Stat(path.Join(repository.RootPath, "v2"))
	assert.NoError(t, err)
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
	_, err = os.Stat(path.Join(repository.RootPath, "v2"))
	assert.NoError(t, err)
}

func TestSetExperimentBeforeStable(t *testing.T) {
	dir := t.TempDir()
	repository := Repository{
		RootPath: dir,
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
	_, err = os.Stat(path.Join(repository.RootPath, "v1"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(path.Join(repository.RootPath, "v2"))
	assert.NoError(t, err)
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
	_, err = os.Stat(path.Join(repository.RootPath, "v2"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestDeleteExperimentWithoutExperiment(t *testing.T) {
	dir := t.TempDir()
	repository := createTestRepository(t, dir, "v1")

	err := repository.DeleteExperiment()
	assert.NoError(t, err)
}
