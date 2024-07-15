// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testCtx = context.TODO()

func newTestRepositories(t *testing.T) *Repositories {
	rootPath := t.TempDir()
	locksRootPath := t.TempDir()
	repositories := NewRepositories(rootPath, locksRootPath)
	return repositories
}

func TestRepositoriesEmpty(t *testing.T) {
	repositories := newTestRepositories(t)

	state, err := repositories.GetState()
	assert.NoError(t, err)
	assert.Empty(t, state)
}

func TestRepositories(t *testing.T) {
	repositories := newTestRepositories(t)

	err := repositories.Create(testCtx, "repo1", "v1", t.TempDir())
	assert.NoError(t, err)
	repository := repositories.Get("repo1")
	err = repository.SetExperiment(testCtx, "v2", t.TempDir())
	assert.NoError(t, err)
	err = repositories.Create(testCtx, "repo2", "v1.0", t.TempDir())
	assert.NoError(t, err)

	state, err := repositories.GetState()
	assert.NoError(t, err)
	assert.Len(t, state, 2)
	assert.Equal(t, state["repo1"], State{Stable: "v1", Experiment: "v2"})
	assert.Equal(t, state["repo2"], State{Stable: "v1.0"})
}

func TestRepositoriesReopen(t *testing.T) {
	repositories := newTestRepositories(t)
	err := repositories.Create(testCtx, "repo1", "v1", t.TempDir())
	assert.NoError(t, err)
	err = repositories.Create(testCtx, "repo2", "v1", t.TempDir())
	assert.NoError(t, err)

	repositories = NewRepositories(repositories.rootPath, repositories.locksPath)

	state, err := repositories.GetState()
	assert.NoError(t, err)
	assert.Len(t, state, 2)
	assert.Equal(t, state["repo1"], State{Stable: "v1"})
	assert.Equal(t, state["repo2"], State{Stable: "v1"})
}

func TestRepositoriesCleanup(t *testing.T) {
	repositories := newTestRepositories(t)

	// repo1 has only a stable
	err := repositories.Create(testCtx, "repo1", "v1", t.TempDir())
	assert.NoError(t, err)

	// repo2 has no stable and should be fully cleaned up
	err = repositories.Create(testCtx, "repo2", "v1", t.TempDir())
	assert.NoError(t, err)
	err = os.Remove(path.Join(repositories.Get("repo2").rootPath, "stable"))
	assert.NoError(t, err)

	// Unrelated dir that shouldn't even be loaded
	err = os.MkdirAll(path.Join(repositories.rootPath, ".ssh"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(path.Join(repositories.rootPath, ".ssh", "known_hosts"), []byte("content"), 0644)
	assert.NoError(t, err)

	err = repositories.Cleanup(testCtx)
	assert.NoError(t, err)

	state, err := repositories.GetState()
	assert.NoError(t, err)
	assert.Len(t, state, 1)
	assert.Equal(t, state["repo1"], State{Stable: "v1"})
	assert.Equal(t, state["repo2"], State{})

	// No side effect: unrelated dir shouldn't be cleaned up
	assert.FileExists(t, path.Join(repositories.rootPath, ".ssh", "known_hosts"))
}
