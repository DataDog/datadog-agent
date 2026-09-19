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

// Both ends of copyFile live in dd-agent-owned trees: the source in the DDOT extension
// directory of the package, the destination in the Agent configuration directory.

func TestCopyFileRefusesSymlinkSrc(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "root-only-secret\n")
	src := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.Symlink(victim, src))

	dst := filepath.Join(cfgDir, "otel-config.yaml")
	require.ErrorContains(t, copyFile(src, dst, 0o640), "could not read "+src)

	written, err := os.ReadFile(dst)
	if err == nil {
		assert.NotContains(t, string(written), "root-only-secret")
	}
}

func TestCopyFileRefusesSymlinkDst(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "victim content\n")
	src := filepath.Join(cfgDir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(src, []byte("template\n"), 0o644))

	dst := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.Symlink(victim, dst))

	require.ErrorContains(t, copyFile(src, dst, 0o640), "could not write "+dst)

	content, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "victim content\n", string(content))
}

func TestCopyFileCopiesRegularFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "otel-config.yaml.example")
	require.NoError(t, os.WriteFile(src, []byte("template\n"), 0o644))

	dst := filepath.Join(dir, "otel-config.yaml")
	require.NoError(t, copyFile(src, dst, 0o640))

	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "template\n", string(content))
	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}
