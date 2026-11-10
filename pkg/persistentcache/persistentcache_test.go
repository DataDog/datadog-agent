// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package persistentcache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePersistentCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)
	err := Write("mykey", "myvalue")
	assert.Nil(t, err)
	value, err := Read("mykey")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)
	value, err = Read("myotherkey")
	assert.Equal(t, "", value)
	assert.Nil(t, err)
}

func TestWriteAtomicPersistentCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	runDir := filepath.Join(testDir, "run")
	mockConfig.SetInTest("run_path", runDir)

	// Subsequent writes replace the value without leaving temporary files.
	key := "my:key"
	require.NoError(t, WriteAtomic(key, "first"))
	require.NoError(t, WriteAtomic(key, "second"))
	value, err := Read(key)
	require.NoError(t, err)
	assert.Equal(t, "second", value)
	files, err := os.ReadDir(filepath.Join(runDir, "my"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "key", files[0].Name())
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0600))
	require.NoError(t, os.WriteFile(source, []byte("new"), 0600))

	require.NoError(t, replaceFile(source, target))
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.Error(t, replaceFile(source, target))
}

func TestWriteAtomicPersistentCacheCleansUpAfterFailedReplacement(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run")
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", runDir)
	require.NoError(t, os.MkdirAll(filepath.Join(runDir, "blocked"), 0700))

	require.Error(t, WriteAtomic("blocked", "value"))
	files, err := os.ReadDir(runDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "blocked", files[0].Name())
}

func TestWriteAtomicPersistentCacheWindowsLongPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path handling")
	}
	runDir := t.TempDir()
	for len(runDir) < 260 {
		runDir = filepath.Join(runDir, strings.Repeat("a", 32))
		require.NoError(t, os.Mkdir(runDir, 0700))
	}
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", runDir)
	key := "my:key"
	path, err := GetFileForKey(key)
	require.NoError(t, err)
	require.Greater(t, len(path), 260)

	require.NoError(t, WriteAtomic(key, "old"))
	require.NoError(t, WriteAtomic(key, "new"))
	value, err := Read(key)
	require.NoError(t, err)
	assert.Equal(t, "new", value)
}

func TestWritePersistentCacheColons(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)
	err := Write("my:key", "myvalue")
	assert.Nil(t, err)
	value, err := Read("my:key")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)

	expectPath := filepath.Join(testDir, "my")
	_, err = os.Stat(expectPath)
	require.Nil(t, err)

	expectPathFile := filepath.Join(testDir, "my", "key")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)
}

func TestWritePersistentCacheInvalidChar(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)
	err := Write("my/key", "myvalue")
	assert.Nil(t, err)
	value, err := Read("my/key")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)

	expectPathFile := filepath.Join(testDir, "mykey")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my:key*foo", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "my", "keyfoo")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my\\key", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "mykey")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my|di*r:key<foo", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "mydir", "keyfoo")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("key_foo-bar", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "key_foo-bar")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)
}

func TestExistsPersistentCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)

	err := Write("mykey", "myvalue")
	assert.Nil(t, err)

	assert.True(t, Exists("mykey"))
	assert.False(t, Exists("myotherkey"))
}

func TestRenamePersistentCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("run_path", testDir)

	err := Write("mykey", "myvalue")
	assert.Nil(t, err)

	err = Rename("mykey", "myotherkey")
	assert.Nil(t, err)

	assert.False(t, Exists("mykey"))
	assert.True(t, Exists("myotherkey"))
}
