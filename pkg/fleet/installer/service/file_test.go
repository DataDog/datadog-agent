// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	originalContent    = "original content"
	transformedContent = "transformed content"
)

var (
	transformFunc = func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte(transformedContent), nil
	}
	failFunc = func() error { return errors.New("fail") }
)

func TestFileTransformWithRollback(t *testing.T) {
	tmpDir := t.TempDir()

	originalPath := tmpDir + "/original.txt"
	mode := os.FileMode(0740)
	require.Nil(t, os.WriteFile(originalPath, []byte(originalContent), mode))

	mutator := newFileMutator(originalPath, transformFunc, nil, nil)

	rollback, err := mutator.mutate(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, rollback)

	assertFile(t, originalPath, transformedContent, mode)

	assert.Nil(t, rollback())
	assertFile(t, originalPath, originalContent, mode)
}

func TestNoChangesNeeded(t *testing.T) {
	tmpDir := t.TempDir()

	originalPath := tmpDir + "/original.txt"
	mode := os.FileMode(0740)
	require.Nil(t, os.WriteFile(originalPath, []byte(originalContent), mode))

	mutator := newFileMutator(originalPath, func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte(originalContent), nil
	}, nil, nil)

	rollback, err := mutator.mutate(context.TODO())
	require.Nil(t, rollback())
	require.NoError(t, err)
	assertFile(t, originalPath, originalContent, mode)
}

func TestFileTransformWithRollback_No_original(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := tmpDir + "/original.txt"

	mutator := newFileMutator(originalPath, transformFunc, nil, nil)

	rollback, err := mutator.mutate(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, rollback)

	assertFile(t, originalPath, transformedContent, detectDefaultMode(t))

	assert.Nil(t, rollback())
	assertNoExists(t, originalPath)
}

func detectDefaultMode(t *testing.T) os.FileMode {
	tmpDir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(tmpDir, "find_mode"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	require.NoError(t, err)
	defer f.Close()
	fileInfo, err := f.Stat()
	require.NoError(t, err)
	return fileInfo.Mode()
}

func TestFileMutator_RollbackOnValidation(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := tmpDir + "/original.txt"
	mode := os.FileMode(0700)
	os.WriteFile(originalPath, []byte(originalContent), mode)

	mutator := newFileMutator(originalPath, transformFunc, nil, failFunc)

	_, err := mutator.mutate(context.TODO())
	require.Error(t, err)

	// check already rolled back
	assertFile(t, originalPath, originalContent, mode)
}

func TestFileTransform_RollbackOnValidation_No_original(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := tmpDir + "/original.txt"

	mutator := newFileMutator(originalPath, transformFunc, nil, failFunc)

	_, err := mutator.mutate(context.TODO())
	require.Error(t, err)

	assertNoExists(t, originalPath)
}

func assertNoExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func assertFile(t *testing.T, path, expectedContent string, expectedMode os.FileMode) {
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expectedContent, string(content))

	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	mode := fileInfo.Mode()
	require.Equal(t, expectedMode, mode)
}

func TestCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := tmpDir + "/original.txt"
	mode := fs.FileMode(0740)
	os.WriteFile(originalPath, []byte(originalContent), mode)
	mutator := newFileMutator(originalPath, nil, nil, nil)
	os.WriteFile(mutator.pathTmp, []byte(originalContent), mode)
	os.WriteFile(mutator.pathBackup, []byte(originalContent), mode)
	assert.FileExists(t, mutator.pathTmp)
	assert.FileExists(t, mutator.pathBackup)
	assert.FileExists(t, mutator.path)

	mutator.cleanup()
	assert.FileExists(t, mutator.path)
	assert.NoFileExists(t, mutator.pathTmp)
	assert.NoFileExists(t, mutator.pathBackup)
}
