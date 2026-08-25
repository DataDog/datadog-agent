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

func TestLoadPackage_WithFlatExtractedDependencies(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	artifactDirectory := writeManifest(t, validManifest+`
dependencies:
  - name: helm
    version: "3.17.2"
  - name: jq
    version: "1.7.1"
`)
	scriptDir := filepath.Join(artifactDirectory, scriptDirectory)
	commandPath := filepath.Join(scriptDir, "run.sh")
	require.NoError(t, os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "helm"), []byte("helm"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "jq"), []byte("jq"), 0o755))
	descriptor := Descriptor{
		Package: fqn,
		Version: "0.0.1",
		SHA256:  "sha256",
	}

	pkg, err := LoadPackage(fqn, descriptor, LocalArtifact{Directory: artifactDirectory})

	require.NoError(t, err)
	assert.Equal(t, []string{commandPath}, pkg.Command)
	assert.Equal(t, []string{
		filepath.Join(scriptDir, "helm"),
		filepath.Join(scriptDir, "jq"),
	}, pkg.ToolPaths)
}

func TestLoadPackage_RejectsEscapingSymlinkCommand(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	artifactDirectory := writeManifest(t, validManifest)
	scriptDir := filepath.Join(artifactDirectory, scriptDirectory)
	externalDirectory := t.TempDir()
	externalCommand := filepath.Join(externalDirectory, "run.sh")
	require.NoError(t, os.WriteFile(externalCommand, []byte("#!/bin/sh\n"), 0o755))
	if err := os.Symlink(externalCommand, filepath.Join(scriptDir, "run.sh")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	descriptor := Descriptor{Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, LocalArtifact{Directory: artifactDirectory})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authored-script command")
}

func TestLoadPackage_RejectsCommandPathTraversal(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	manifest := strings.Replace(validManifest, `command: ["run.sh"]`, `command: ["../run.sh"]`, 1)
	artifactDirectory := writeManifest(t, manifest)
	require.NoError(t, os.WriteFile(filepath.Join(artifactDirectory, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	descriptor := Descriptor{Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, LocalArtifact{Directory: artifactDirectory})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "command path")
}

func TestLoadPackage_RejectsDependencyPathComponents(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	artifactDirectory := writeManifest(t, validManifest+`
dependencies:
  - name: ../helm
    version: "3.17.2"
`)
	scriptDir := filepath.Join(artifactDirectory, scriptDirectory)
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(artifactDirectory, "helm"), []byte("helm"), 0o755))
	descriptor := Descriptor{Package: fqn, Version: "0.0.1"}

	_, err := LoadPackage(fqn, descriptor, LocalArtifact{Directory: artifactDirectory})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency name")
}

func TestValidatePackageIdentity(t *testing.T) {
	const fqn = "com.datadoghq.authoredscripts.echo"
	tests := []struct {
		name        string
		mutate      func(*Descriptor, *Manifest)
		expectError string
	}{
		{
			name: "descriptor package mismatch",
			mutate: func(descriptor *Descriptor, _ *Manifest) {
				descriptor.Package = "com.datadoghq.authoredscripts.other"
			},
			expectError: "descriptor package",
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
		{name: "valid identity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := Descriptor{Package: fqn, Version: "0.0.1"}
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
