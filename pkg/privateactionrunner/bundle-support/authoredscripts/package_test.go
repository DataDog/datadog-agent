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

func TestLoadPackage_WithIsolatedDependencies(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	contents := `
{
  "schema-version": "v1",
  "version": "0.0.1",
  "fqn": "com.datadoghq.authoredscripts.echo",
  "command": {"entrypoint": "run.sh"},
  "allowedEnvVars": ["HOME"],
  "dependencies": [
    {"name": "helm", "version": "3.17.2"},
    {"name": "jq", "version": "1.7.1"}
  ]
}
`
	artifact := writePackageManifest(t, contents)
	commandPath := filepath.Join(artifact.ScriptDirectory(), "run.sh")
	require.NoError(t, os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.MkdirAll(artifact.DependencyDirectory("helm"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(artifact.DependencyDirectory("helm"), "helm"), []byte("helm"), 0o755))
	require.NoError(t, os.MkdirAll(artifact.DependencyDirectory("jq"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(artifact.DependencyDirectory("jq"), "jq"), []byte("jq"), 0o755))
	descriptor := Descriptor{
		FQN:     fqn,
		Package: fqn,
		Version: "0.0.1",
		SHA256:  "sha256",
	}

	pkg, err := LoadPackage(fqn, descriptor, artifact)

	require.NoError(t, err)
	assert.Equal(t, []string{commandPath}, pkg.Command)
	assert.Equal(t, []string{
		artifact.DependencyDirectory("helm"),
		artifact.DependencyDirectory("jq"),
	}, pkg.ExecutableDirectories)
}

func TestLoadPackage_RejectsEscapingSymlinkCommand(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	artifact := writePackageManifest(t, validManifest)
	externalDirectory := t.TempDir()
	externalCommand := filepath.Join(externalDirectory, "run.sh")
	require.NoError(t, os.WriteFile(externalCommand, []byte("#!/bin/sh\n"), 0o755))
	if err := os.Symlink(externalCommand, filepath.Join(artifact.ScriptDirectory(), "run.sh")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	descriptor := Descriptor{FQN: fqn, Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, artifact)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authored-script command")
}

func TestLoadPackage_RejectsCommandPathTraversal(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	manifest := strings.Replace(validManifest, `"entrypoint": "run.sh"`, `"entrypoint": "../run.sh"`, 1)
	artifact := writePackageManifest(t, manifest)
	require.NoError(t, os.WriteFile(filepath.Join(artifact.Directory, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	descriptor := Descriptor{FQN: fqn, Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, artifact)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authored-script command")
}

func TestLoadPackage_RejectsDependencyPathComponents(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	contents := `
{
  "schema-version": "v1",
  "version": "0.0.1",
  "fqn": "com.datadoghq.authoredscripts.echo",
  "command": {"entrypoint": "run.sh"},
  "allowedEnvVars": ["HOME"],
  "dependencies": [
    {"name": "../helm", "version": "3.17.2"}
  ]
}
`
	artifact := writePackageManifest(t, contents)
	require.NoError(t, os.WriteFile(filepath.Join(artifact.ScriptDirectory(), "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	descriptor := Descriptor{FQN: fqn, Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, artifact)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency name")
}

func writePackageManifest(t *testing.T, contents string) LocalArtifact {
	t.Helper()
	artifact := LocalArtifact{Directory: t.TempDir()}
	require.NoError(t, os.MkdirAll(artifact.ScriptDirectory(), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(artifact.ScriptDirectory(), manifestFile), []byte(contents), 0o644))
	return artifact
}

func TestValidatePackageIdentity(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echoAction"
	tests := []struct {
		name        string
		mutate      func(*Descriptor, *Manifest)
		expectError string
	}{
		{
			name: "descriptor FQN mismatch",
			mutate: func(descriptor *Descriptor, _ *Manifest) {
				descriptor.FQN = "com.datadoghq.authoredscripts.other"
			},
			expectError: "descriptor FQN",
		},
		{
			name: "manifest FQN mismatch",
			mutate: func(_ *Descriptor, manifest *Manifest) {
				manifest.FQN = "com.datadoghq.authoredscripts.other"
			},
			expectError: "manifest FQN",
		},
		{
			name: "manifest version mismatch",
			mutate: func(_ *Descriptor, manifest *Manifest) {
				manifest.Version = "0.0.2"
			},
			expectError: "manifest version",
		},
		{
			name: "FQN casing differs",
			mutate: func(descriptor *Descriptor, manifest *Manifest) {
				descriptor.FQN = strings.ToLower(descriptor.FQN)
				manifest.FQN = strings.ToUpper(manifest.FQN)
			},
		},
		{name: "valid identity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := Descriptor{FQN: fqn, Package: "com.datadoghq.authoredscripts.echoaction", Version: "0.0.1"}
			manifest := &Manifest{FQN: fqn, Version: descriptor.Version}
			if tt.mutate != nil {
				tt.mutate(&descriptor, manifest)
			}

			err := validatePackageIdentity(fqn, descriptor, manifest)
			if tt.expectError == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}
