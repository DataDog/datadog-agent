// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFQN     = "com.datadoghq.authoredscripts.helm.addRepo"
	testCommand = "run.sh"
)

// testDigest builds a digest that is distinct per seed so that several packages can be
// cached for the same action.
func testDigest(seed string) string {
	return strings.Repeat(seed, 64)[:64]
}

// packageManifest renders a valid manifest for a cached package.
func packageManifest(fqn, version string, dependencies []string) string {
	manifest := &strings.Builder{}
	fmt.Fprintf(manifest, `schema-version: v1
dd-package: dd-par-scripts-test
version: %s
title: Test
description: A cached package.
fqn: %s

config:
  command: [%q]
`, version, fqn, testCommand)

	if len(dependencies) > 0 {
		manifest.WriteString("\ndependencies:\n")
		for _, dependency := range dependencies {
			fmt.Fprintf(manifest, "  - {name: %s, version: 1.0.0}\n", dependency)
		}
	}
	return manifest.String()
}

// writeCachedPackage writes a complete, valid package into the cache and returns its
// directory. Tests that need a broken package start from this and break one thing.
func writeCachedPackage(t *testing.T, cacheRoot, fqn, digest, version string, dependencies ...string) string {
	t.Helper()

	directory := packageDirectory(cacheRoot, fqn, digest, currentPlatform())
	scriptDir := filepath.Join(directory, scriptDirectory)
	require.NoError(t, os.MkdirAll(scriptDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, manifestFile),
		[]byte(packageManifest(fqn, version, dependencies)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(scriptDir, testCommand),
		[]byte("#!/bin/bash\n"), 0o755))

	for _, dependency := range dependencies {
		toolDir := filepath.Join(directory, toolsDirectory, dependency)
		require.NoError(t, os.MkdirAll(toolDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(toolDir, dependency), []byte("binary"), 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(directory, completionMarker), nil, 0o644))
	return directory
}

// fixedResolver resolves every action to the same digest, so that store behaviour can be
// tested independently of how a digest was chosen.
type fixedResolver string

func (r fixedResolver) Resolve(string) (string, error) { return string(r), nil }

func TestOpen_ReturnsCachedPackage(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.2.3", "helm", "jq")

	pkg, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	require.NoError(t, err)
	assert.Equal(t, directory, pkg.Directory)
	assert.Equal(t, digest, pkg.ArtifactDigest)
	assert.Equal(t, testFQN, pkg.Manifest.FQN)
	assert.Equal(t, "1.2.3", pkg.Manifest.Version)
	assert.Equal(t, []string{filepath.Join(directory, scriptDirectory, testCommand)}, pkg.Command)
	assert.Equal(t, []string{
		filepath.Join(directory, toolsDirectory, "helm"),
		filepath.Join(directory, toolsDirectory, "jq"),
	}, pkg.ToolDirectories)
}

func TestOpen_PreservesCommandArguments(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")
	manifest := strings.Replace(packageManifest(testFQN, "1.0.0", nil),
		fmt.Sprintf("command: [%q]", testCommand),
		fmt.Sprintf("command: [%q, \"--flag\", \"value\"]", testCommand), 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, scriptDirectory, manifestFile), []byte(manifest), 0o644))

	pkg, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	require.NoError(t, err)
	assert.Equal(t, []string{
		filepath.Join(directory, scriptDirectory, testCommand), "--flag", "value",
	}, pkg.Command)
}

func TestOpen_PackageWithoutDependenciesHasNoToolDirectories(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")

	pkg, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	require.NoError(t, err)
	assert.Empty(t, pkg.ToolDirectories)
}

func TestOpen_TreatsPackageMissingCompletionMarkerAsNotCached(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")
	require.NoError(t, os.Remove(filepath.Join(directory, completionMarker)))

	_, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestOpen_RejectsPackageDeclaringAnotherAction(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")
	otherDirectory := packageDirectory(cacheRoot, "com.datadoghq.authoredscripts.echo", digest, currentPlatform())
	require.NoError(t, os.MkdirAll(filepath.Dir(otherDirectory), 0o755))
	require.NoError(t, os.Rename(packageDirectory(cacheRoot, testFQN, digest, currentPlatform()), otherDirectory))

	_, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open("com.datadoghq.authoredscripts.echo")

	assert.ErrorContains(t, err, "declares action")
}

func TestOpen_RejectsMissingCommand(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")
	require.NoError(t, os.Remove(filepath.Join(directory, scriptDirectory, testCommand)))

	_, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	assert.ErrorContains(t, err, "could not open authored-script command")
}

func TestOpen_RejectsCommandThatIsNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows files carry no executable bit")
	}
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0")
	require.NoError(t, os.Chmod(filepath.Join(directory, scriptDirectory, testCommand), 0o644))

	_, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	assert.ErrorContains(t, err, "cannot be executed")
}

func TestOpen_RejectsMissingTool(t *testing.T) {
	cacheRoot := t.TempDir()
	digest := testDigest("a")
	directory := writeCachedPackage(t, cacheRoot, testFQN, digest, "1.0.0", "helm")
	require.NoError(t, os.RemoveAll(filepath.Join(directory, toolsDirectory, "helm")))

	_, err := NewLocalStore(cacheRoot, fixedResolver(digest)).Open(testFQN)

	assert.ErrorContains(t, err, `could not access authored-script tool "helm"`)
}

func TestOpen_RejectsInvalidActionName(t *testing.T) {
	tests := []struct {
		name string
		fqn  string
	}{
		{name: "empty", fqn: ""},
		{name: "single segment", fqn: "authoredscripts"},
		{name: "parent directory", fqn: "../../etc/passwd"},
		{name: "path separator", fqn: "com.datadoghq/authoredscripts.echo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLocalStore(t.TempDir(), fixedResolver(testDigest("a"))).Open(tt.fqn)

			assert.ErrorContains(t, err, "is not a valid authored-script action name")
		})
	}
}

func TestOpen_RejectsInvalidDigestFromResolver(t *testing.T) {
	tests := []struct {
		name   string
		digest string
	}{
		{name: "empty", digest: ""},
		{name: "parent directory", digest: "../.."},
		{name: "algorithm prefix", digest: "sha256:" + testDigest("a")},
		{name: "too short", digest: "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLocalStore(t.TempDir(), fixedResolver(tt.digest)).Open(testFQN)

			assert.ErrorContains(t, err, "is not a valid authored-script artifact digest")
		})
	}
}

func TestOpen_PropagatesResolverError(t *testing.T) {
	_, err := NewLocalStore(t.TempDir(), NewCacheResolver(t.TempDir())).Open(testFQN)

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestResolveCommand_RejectsAbsoluteCommand(t *testing.T) {
	_, err := resolveCommand(t.TempDir(), []string{"/bin/sh"})

	assert.ErrorContains(t, err, "must be relative to the package")
}

func TestResolveToolDirectories_RejectsToolNameThatIsAPath(t *testing.T) {
	tests := []struct {
		name string
		tool string
	}{
		{name: "parent directory", tool: ".."},
		{name: "nested path", tool: "helm/bin"},
		{name: "escaping path", tool: "../script"},
		{name: "current directory", tool: "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveToolDirectories(t.TempDir(), []Dependency{{Name: tt.tool, Version: "1.0.0"}})

			assert.ErrorContains(t, err, "is not a valid tool name")
		})
	}
}

func TestCheckComplete_RejectsMarkerThatIsNotAFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, completionMarker), 0o755))

	err := checkComplete(directory)

	assert.ErrorContains(t, err, "is not a regular file")
	assert.False(t, errors.Is(err, ErrNotCached))
}
