// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// os.Root reports only the name it was given, so these helpers have to name the full path
// themselves or an install hook failure is untraceable from the logs.

func TestReadFileInDirErrorNamesOperationAndPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "datadog.yaml")

	_, err := readFileInDir(missing)
	require.ErrorContains(t, err, "could not read "+missing)
	assert.ErrorIs(t, err, os.ErrNotExist, "callers tell a fresh install from a real failure with errors.Is")
}

func TestWriteFileInDirErrorNamesOperationAndPath(t *testing.T) {
	base := t.TempDir()
	cfgDir := filepath.Join(base, "cfg")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	outPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.Symlink(filepath.Join(base, "victim"), outPath))

	err := writeFileInDir(outPath, []byte("config\n"), 0o640)
	require.ErrorContains(t, err, "could not write "+outPath)
}

func TestLstatInDirErrorNamesOperationAndPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "otel-config.yaml")

	_, err := lstatInDir(missing)
	require.ErrorContains(t, err, "could not stat "+missing)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHelpersErrorNamesMissingDirectory(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "absent")
	path := filepath.Join(missingDir, "datadog.yaml")

	_, err := readFileInDir(path)
	require.ErrorContains(t, err, "open "+missingDir,
		"os.OpenRoot names the directory itself, so the helper passes its error through")
	assert.ErrorIs(t, err, os.ErrNotExist,
		"a missing configuration directory must stay detectable as ErrNotExist")
}
