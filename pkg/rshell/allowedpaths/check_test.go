// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPath(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "readonly"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "existing"), []byte("preserve"), 0600))
	sandbox, _, err := New([]string{root + ":rw", filepath.Join(root, "readonly") + ":ro"})
	require.NoError(t, err)
	defer sandbox.Close()
	sandbox.SetWritable()
	for _, operation := range []PathOperation{PathRead, PathWrite, PathRemove} {
		assert.NoError(t, sandbox.CheckPath("existing", root, operation))
		assert.NoError(t, sandbox.CheckPath("missing", root, operation))
		assert.ErrorIs(t, sandbox.CheckPath(filepath.Join(outside, "secret"), root, operation), os.ErrPermission)
		assert.ErrorIs(t, sandbox.CheckPath(filepath.Join("..", "secret"), root, operation), os.ErrPermission)
		var empty *Sandbox
		assert.ErrorIs(t, empty.CheckPath("existing", root, operation), os.ErrPermission)
	}
	assert.NoError(t, sandbox.CheckPath(filepath.Join("readonly", "file"), root, PathRead))
	assert.ErrorIs(t, sandbox.CheckPath(filepath.Join("readonly", "file"), root, PathWrite), os.ErrPermission)
	assert.ErrorIs(t, sandbox.CheckPath(filepath.Join("readonly", "file"), root, PathRemove), os.ErrPermission)
	assert.Error(t, sandbox.CheckPath(".", root, PathWrite))
	assert.Error(t, sandbox.CheckPath(".", root, PathRemove))
	assert.Error(t, sandbox.CheckPath("existing", root, PathOperation("invalid")))
	contents, err := os.ReadFile(filepath.Join(root, "existing"))
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(contents))
	_, err = os.Stat(filepath.Join(root, "missing"))
	assert.True(t, os.IsNotExist(err))
}

func TestCheckPathSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, []byte("preserve"), 0600))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	require.NoError(t, os.Symlink("file", filepath.Join(root, "link")))
	sandbox, _, err := New([]string{root + ":rw"})
	require.NoError(t, err)
	defer sandbox.Close()
	sandbox.SetWritable()
	assert.ErrorIs(t, sandbox.CheckPath("escape", root, PathRead), os.ErrPermission)
	assert.ErrorIs(t, sandbox.CheckPath("escape", root, PathStat), os.ErrPermission)
	assert.NoError(t, sandbox.CheckPath("escape", root, PathLstat))
	assert.Error(t, sandbox.CheckPath("escape", root, PathWrite))
	assert.NoError(t, sandbox.CheckPath("escape", root, PathRemove), "unlink checks the link, not its target")
	assert.NoError(t, sandbox.CheckPath("link", root, PathRead))
	assert.Error(t, sandbox.CheckPath("link", root, PathWrite))
	assert.NoError(t, sandbox.CheckPath("link", root, PathRemove))
	for i := 0; i < 12; i++ {
		target := "file"
		if i > 0 {
			target = fmt.Sprintf("chain-%d", i-1)
		}
		require.NoError(t, os.Symlink(target, filepath.Join(root, fmt.Sprintf("chain-%d", i))))
	}
	assert.NoError(t, sandbox.CheckPath("chain-11", root, PathRead))
	shared, _, err := New([]string{root, outside})
	require.NoError(t, err)
	defer shared.Close()
	assert.NoError(t, shared.CheckPath("escape", root, PathRead))
	assert.NoError(t, sandbox.CheckPath("link", root, PathRead))
	_, err = os.Lstat(filepath.Join(root, "escape"))
	assert.NoError(t, err, "checking remove must leave the symlink intact")
}

func TestCheckPathHardLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("write link-count guard is intentionally unavailable on Windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(outside, "original")
	require.NoError(t, os.WriteFile(file, []byte("preserve"), 0600))
	if err := os.Link(file, filepath.Join(root, "alias")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	sandbox, _, err := New([]string{root + ":rw"})
	require.NoError(t, err)
	defer sandbox.Close()
	sandbox.SetWritable()
	assert.NoError(t, sandbox.CheckPath("alias", root, PathRead))
	assert.ErrorIs(t, sandbox.CheckPath("alias", root, PathWrite), ErrMultiplyLinkedWriteTarget)
	assert.NoError(t, sandbox.CheckPath("alias", root, PathRemove))
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(data))
}
