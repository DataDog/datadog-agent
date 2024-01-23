// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createLink(t *testing.T, linkPath string, targetPath string) {
	err := linkSet(linkPath, targetPath)
	assert.NoError(t, err)
}

func createTarget(t *testing.T, targetPath string) {
	err := os.Mkdir(targetPath, 0755)
	assert.NoError(t, err)
}

func TestLinkRead(t *testing.T) {
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "link")
	targetPath := filepath.Join(tmpDir, "target")
	createTarget(t, targetPath)
	createLink(t, linkPath, targetPath)

	actualTargetPath, err := linkRead(linkPath)
	assert.NoError(t, err)

	// the following cleanup is required on darwin because t.TempDir returns a symlinked path.
	// see https://github.com/golang/go/issues/56259
	targetPath, err = filepath.EvalSymlinks(targetPath)
	assert.NoError(t, err)
	actualTargetPath, err = filepath.EvalSymlinks(actualTargetPath)
	assert.NoError(t, err)

	assert.Equal(t, targetPath, actualTargetPath)
}

func TestLinkExists(t *testing.T) {
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "link")
	targetPath := filepath.Join(tmpDir, "target")

	exists, err := linkExists(linkPath)
	assert.NoError(t, err)
	assert.False(t, exists)

	createTarget(t, targetPath)
	createLink(t, linkPath, targetPath)

	exists, err = linkExists(linkPath)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestLinkSet(t *testing.T) {
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "link")
	targetPath := filepath.Join(tmpDir, "target")
	createTarget(t, targetPath)

	err := linkSet(linkPath, targetPath)
	assert.NoError(t, err)

	exists, err := linkExists(linkPath)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestLinkDelete(t *testing.T) {
	tmpDir := t.TempDir()
	linkPath := filepath.Join(tmpDir, "link")
	targetPath := filepath.Join(tmpDir, "target")
	createTarget(t, targetPath)
	createLink(t, linkPath, targetPath)

	err := linkDelete(linkPath)
	assert.NoError(t, err)

	exists, err := linkExists(linkPath)
	assert.NoError(t, err)
	assert.False(t, exists)
}
