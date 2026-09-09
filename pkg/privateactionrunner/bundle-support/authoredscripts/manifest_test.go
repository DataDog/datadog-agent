// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validManifest = `
schema-version: v1
dd-package: dd-par-scripts-echo
version: 0.0.1
title: Echo
description: Prints a message to stdout.
fqn: com.datadoghq.authoredscripts.echo
config:
  command: ["run.sh"]
  allowedEnvVars: ["HOME"]
`

func writeManifest(t *testing.T, contents string) string {
	t.Helper()
	artifactDirectory := t.TempDir()
	scriptDir := filepath.Join(artifactDirectory, scriptDirectory)
	require.NoError(t, os.MkdirAll(scriptDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, manifestFile), []byte(contents), 0o644))
	return artifactDirectory
}

func TestLoadManifest_Valid(t *testing.T) {
	artifactDirectory := writeManifest(t, validManifest)

	manifest, err := loadManifest(artifactDirectory)

	require.NoError(t, err)
	assert.Equal(t, "v1", manifest.SchemaVersion)
	assert.Equal(t, "dd-par-scripts-echo", manifest.Package)
	assert.Equal(t, []string{"run.sh"}, manifest.Config.Command)
	assert.Equal(t, []string{"HOME"}, manifest.Config.AllowedEnvVars)
}

func TestLoadManifest_WithSessionEnvVars(t *testing.T) {
	artifactDirectory := writeManifest(t, validManifest+`
  setSessionEnvVars:
    - name: SESSION_EXAMPLE_VALUE
      value: example-session-value
      kind: value
`)

	manifest, err := loadManifest(artifactDirectory)

	require.NoError(t, err)
	require.Len(t, manifest.Config.SetSessionEnvVars, 1)
	assert.Equal(t, environmentKindValue, manifest.Config.SetSessionEnvVars[0].Kind)
}

func TestLoadManifest_MissingManifestFile(t *testing.T) {
	artifactDirectory := t.TempDir()

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
}

func TestLoadManifest_RejectsUnknownField(t *testing.T) {
	artifactDirectory := writeManifest(t, validManifest+"\nunexpectedField: true\n")

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
}

func TestLoadManifest_RejectsMultipleDocuments(t *testing.T) {
	artifactDirectory := writeManifest(t, validManifest+"\n---\nschema-version: v1\n")

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one YAML document")
}

func TestLoadManifest_RejectsOversizedManifest(t *testing.T) {
	padding := strings.Repeat("a", maxManifestSize+1)
	artifactDirectory := writeManifest(t, validManifest+"\n# "+padding+"\n")

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "byte limit")
}

func TestValidateManifest(t *testing.T) {
	validCommand := []string{"run.sh"}

	tests := []struct {
		name        string
		manifest    *Manifest
		expectError string
	}{
		{
			name: "unsupported schema version",
			manifest: &Manifest{
				SchemaVersion: "v2",
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: validCommand},
			},
			expectError: "unsupported authored-script manifest schema version",
		},
		{
			name: "missing package",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: validCommand},
			},
			expectError: "package is required",
		},
		{
			name: "missing version",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: validCommand},
			},
			expectError: "version is required",
		},
		{
			name: "missing fqn",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				Config:        ScriptConfig{Command: validCommand},
			},
			expectError: "FQN is required",
		},
		{
			name: "missing command",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
			},
			expectError: "command is required",
		},
		{
			name: "empty first command argument",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: []string{""}},
			},
			expectError: "command is required",
		},
		{
			name: "unsupported session env var kind",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config: ScriptConfig{
					Command:           validCommand,
					SetSessionEnvVars: []EnvironmentVariable{{Name: "X", Value: "y", Kind: "socket"}},
				},
			},
			expectError: "session environment variable",
		},
		{
			name: "incomplete session env var",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config: ScriptConfig{
					Command:           validCommand,
					SetSessionEnvVars: []EnvironmentVariable{{Name: "X", Kind: environmentKindValue}},
				},
			},
			expectError: "session environment variables require",
		},
		{
			name: "dependency missing version",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: validCommand},
				Dependencies:  []Dependency{{Name: "jq"}},
			},
			expectError: "dependencies require a name and version",
		},
		{
			name: "valid manifest",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Package:       "dd-par-scripts-echo",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Config:        ScriptConfig{Command: validCommand},
				Dependencies:  []Dependency{{Name: "jq", Version: "1.7.1"}},
			},
			expectError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(tt.manifest)
			if tt.expectError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestOpenPackageFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "subdir"), 0o755))

	t.Run("regular file opens", func(t *testing.T) {
		file, err := openPackageFile(root, "file.txt")
		require.NoError(t, err)
		defer file.Close()
		assert.Equal(t, filepath.Join(root, "file.txt"), file.Name())
	})

	t.Run("empty path rejected", func(t *testing.T) {
		_, err := openPackageFile(root, "")
		require.Error(t, err)
	})

	t.Run("parent traversal rejected", func(t *testing.T) {
		_, err := openPackageFile(root, "../file.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not relative to the package")
	})

	t.Run("missing file rejected", func(t *testing.T) {
		_, err := openPackageFile(root, "missing.txt")
		require.Error(t, err)
	})

	t.Run("directory rejected", func(t *testing.T) {
		_, err := openPackageFile(root, "subdir")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a regular file")
	})
}
