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

func newTestSession(t *testing.T) *Session {
	t.Helper()
	session, err := NewSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Cleanup() })
	return session
}

func lookupEnv(t *testing.T, environment []string, name string) (string, bool) {
	t.Helper()
	prefix := name + "="
	for _, entry := range environment {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			return value, true
		}
	}
	return "", false
}

func TestBuildEnvironment_ManagedVariables(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{}}

	environment, err := pkg.BuildEnvironment(session, nil)

	require.NoError(t, err)
	home, ok := lookupEnv(t, environment, "HOME")
	require.True(t, ok)
	assert.Equal(t, session.HomeDirectory, home)
	tmpdir, ok := lookupEnv(t, environment, "TMPDIR")
	require.True(t, ok)
	assert.Equal(t, session.TempDirectory, tmpdir)
	path, ok := lookupEnv(t, environment, "PATH")
	require.True(t, ok)
	assert.Equal(t, defaultExecutablePath, path)
}

func TestBuildEnvironment_RejectsManagedAllowedEnvVar(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{AllowedEnvVars: []string{"PATH"}}}}

	_, err := pkg.BuildEnvironment(session, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "managed by PAR")
}

func TestBuildEnvironment_PassesThroughAllowedEnvVar(t *testing.T) {
	t.Setenv("MY_TOKEN", "secret-value")
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{AllowedEnvVars: []string{"MY_TOKEN"}}}}

	environment, err := pkg.BuildEnvironment(session, nil)

	require.NoError(t, err)
	value, ok := lookupEnv(t, environment, "MY_TOKEN")
	require.True(t, ok)
	assert.Equal(t, "secret-value", value)
}

func TestBuildEnvironment_SessionEnvVarRejectsPathOverride(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "PATH", Value: "/extra/bin", Kind: environmentKindValue}},
	}}}

	_, err := pkg.BuildEnvironment(session, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override the managed")
}

func TestBuildEnvironment_SessionEnvVarRejectsHomeOverride(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "HOME", Value: "/custom/home", Kind: environmentKindValue}},
	}}}

	_, err := pkg.BuildEnvironment(session, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override the managed")
}

func TestBuildEnvironment_SessionEnvVarRejectsTmpdirOverride(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "TMPDIR", Value: "/custom/tmp", Kind: environmentKindValue}},
	}}}

	_, err := pkg.BuildEnvironment(session, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot override the managed")
}

func TestBuildEnvironment_SessionEnvVarFileIsCreatedUnderSessionRoot(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "OUTPUT_FILE", Value: "output/result.json", Kind: environmentKindFile}},
	}}}

	environment, err := pkg.BuildEnvironment(session, nil)

	require.NoError(t, err)
	value, ok := lookupEnv(t, environment, "OUTPUT_FILE")
	require.True(t, ok)
	expectedPath := filepath.Join(session.RootDirectory, "output/result.json")
	assert.Equal(t, expectedPath, value)
	info, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
}

func TestBuildEnvironment_SessionEnvVarDirectoryIsCreatedUnderSessionRoot(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "WORKDIR", Value: "workdir", Kind: environmentKindDirectory}},
	}}}

	environment, err := pkg.BuildEnvironment(session, nil)

	require.NoError(t, err)
	value, ok := lookupEnv(t, environment, "WORKDIR")
	require.True(t, ok)
	expectedPath := filepath.Join(session.RootDirectory, "workdir")
	assert.Equal(t, expectedPath, value)
	info, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestBuildEnvironment_SessionEnvVarRejectsPathTraversal(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{Manifest: &Manifest{Config: ScriptConfig{
		SetSessionEnvVars: []EnvironmentVariable{{Name: "OUTPUT_FILE", Value: "../escape.json", Kind: environmentKindFile}},
	}}}

	_, err := pkg.BuildEnvironment(session, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not relative to its session directory")
}

func TestBuildExecutablePath_DedupsDirectories(t *testing.T) {
	path, err := buildExecutablePath([]string{
		"/tools/a/helm",
		"/tools/a/jq",
		"/tools/b/kubectl",
	})

	require.NoError(t, err)
	assert.Equal(t, "/tools/a"+string(os.PathListSeparator)+"/tools/b"+string(os.PathListSeparator)+defaultExecutablePath, path)
}

func TestBuildExecutablePath_RejectsPathSeparatorInDirectory(t *testing.T) {
	directory := "/tools" + string(os.PathListSeparator) + "a"

	_, err := buildExecutablePath([]string{directory + "/helm"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separator")
}

func TestMaterializeEnvironmentVariable_Value(t *testing.T) {
	root := t.TempDir()

	value, err := materializeEnvironmentVariable(root, "session", EnvironmentVariable{Name: "X", Value: "plain-value", Kind: environmentKindValue})

	require.NoError(t, err)
	assert.Equal(t, "plain-value", value)
}

func TestMaterializeEnvironmentVariable_RejectsRoot(t *testing.T) {
	root := t.TempDir()

	_, err := materializeEnvironmentVariable(root, "session", EnvironmentVariable{Name: "X", Value: ".", Kind: environmentKindDirectory})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use its session directory")
}

func TestMaterializeEnvironmentVariable_FileAlreadyExistsIsReused(t *testing.T) {
	root := t.TempDir()
	variable := EnvironmentVariable{Name: "X", Value: "state.json", Kind: environmentKindFile}

	first, err := materializeEnvironmentVariable(root, "session", variable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first, []byte("data"), 0o600))

	second, err := materializeEnvironmentVariable(root, "session", variable)

	require.NoError(t, err)
	assert.Equal(t, first, second)
	contents, err := os.ReadFile(second)
	require.NoError(t, err)
	assert.Equal(t, "data", string(contents))
}

func TestMaterializeEnvironmentVariable_RejectsUnsupportedKind(t *testing.T) {
	root := t.TempDir()

	_, err := materializeEnvironmentVariable(root, "session", EnvironmentVariable{Name: "X", Value: "socket-path", Kind: "socket"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported kind")
}

func TestParameterEnvName(t *testing.T) {
	assert.Equal(t, "PAR_ENV_TARGET_URL", parameterEnvName("targetURL"))
}

func TestAddParameterEnvironment_RejectsNameCollision(t *testing.T) {
	err := addParameterEnvironment(map[string]string{}, map[string]interface{}{
		"targetURL": "a",
		"TargetURL": "b",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "PAR_ENV_TARGET_URL")
}
