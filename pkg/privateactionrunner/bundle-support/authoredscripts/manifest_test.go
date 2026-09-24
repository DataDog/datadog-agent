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
{
  "schema-version": "v1",
  "version": "0.0.1",
  "fqn": "com.datadoghq.authoredscripts.echo",
  "command": {"entrypoint": "run.sh"},
  "allowedEnvVars": ["HOME"]
}
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
	assert.Equal(t, "run.sh", manifest.Command.Entrypoint)
	assert.Equal(t, []string{"HOME"}, manifest.AllowedEnvVars)
}

func TestLoadManifest_WithSessionEnvVars(t *testing.T) {
	contents := `
{
  "schema-version": "v1",
  "version": "0.0.1",
  "fqn": "com.datadoghq.authoredscripts.echo",
  "command": {"entrypoint": "run.sh"},
  "allowedEnvVars": ["HOME"],
  "setSessionEnvVars": [
    {"name": "SESSION_EXAMPLE_VALUE", "value": "example-session-value", "kind": "value"}
  ]
}
`
	artifactDirectory := writeManifest(t, contents)

	manifest, err := loadManifest(artifactDirectory)

	require.NoError(t, err)
	require.Len(t, manifest.SetSessionEnvVars, 1)
	assert.Equal(t, environmentKindValue, manifest.SetSessionEnvVars[0].Kind)
}

func TestLoadManifest_MissingManifestFile(t *testing.T) {
	artifactDirectory := t.TempDir()

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
}

func TestLoadManifest_RejectsUnknownField(t *testing.T) {
	contents := `
{
  "schema-version": "v1",
  "version": "0.0.1",
  "fqn": "com.datadoghq.authoredscripts.echo",
  "command": {"entrypoint": "run.sh"},
  "unexpectedField": true
}
`
	artifactDirectory := writeManifest(t, contents)

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
}

func TestLoadManifest_RejectsMultipleDocuments(t *testing.T) {
	artifactDirectory := writeManifest(t, validManifest+"\n{\"schema-version\": \"v1\"}\n")

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one JSON document")
}

func TestLoadManifest_RejectsOversizedManifest(t *testing.T) {
	padding := strings.Repeat("a", maxManifestSize+1)
	artifactDirectory := writeManifest(t, validManifest+"\n"+padding+"\n")

	_, err := loadManifest(artifactDirectory)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "byte limit")
}

func TestValidateManifest(t *testing.T) {
	validCommand := Command{Entrypoint: "run.sh"}

	tests := []struct {
		name        string
		manifest    *Manifest
		expectError string
	}{
		{
			name: "unsupported schema version",
			manifest: &Manifest{
				SchemaVersion: "v2",
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Command:       validCommand,
			},
			expectError: "unsupported authored-script manifest schema version",
		},
		{
			name: "missing version",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				FQN:           "com.datadoghq.authoredscripts.echo",
				Command:       validCommand,
			},
			expectError: "version is required",
		},
		{
			name: "missing fqn",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				Command:       validCommand,
			},
			expectError: "FQN is required",
		},
		{
			name: "missing command",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
			},
			expectError: "command is required",
		},
		{
			name: "empty entrypoint",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Command:       Command{Entrypoint: ""},
			},
			expectError: "command is required",
		},
		{
			name: "unsupported session env var kind",
			manifest: &Manifest{
				SchemaVersion:     manifestSchemaVersion,
				Version:           "0.0.1",
				FQN:               "com.datadoghq.authoredscripts.echo",
				Command:           validCommand,
				SetSessionEnvVars: []EnvironmentVariable{{Name: "X", Value: "y", Kind: "socket"}},
			},
			expectError: "session environment variable",
		},
		{
			name: "incomplete session env var",
			manifest: &Manifest{
				SchemaVersion:     manifestSchemaVersion,
				Version:           "0.0.1",
				FQN:               "com.datadoghq.authoredscripts.echo",
				Command:           validCommand,
				SetSessionEnvVars: []EnvironmentVariable{{Name: "X", Kind: environmentKindValue}},
			},
			expectError: "session environment variables require",
		},
		{
			name: "dependency missing version",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Command:       validCommand,
				Dependencies:  []Dependency{{Name: "jq"}},
			},
			expectError: "dependencies require a name and version",
		},
		{
			name: "valid manifest",
			manifest: &Manifest{
				SchemaVersion: manifestSchemaVersion,
				Version:       "0.0.1",
				FQN:           "com.datadoghq.authoredscripts.echo",
				Command:       validCommand,
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
