// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package overlay

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func init() {
	// mknod is not allowed in userns. This simplifies testing in a container env.
	whiteoutCharDev = 1
}

func TestOverlayBasic(t *testing.T) {
	layer1 := path.Join(t.TempDir(), "1")
	layer2 := path.Join(t.TempDir(), "2")
	layer3 := path.Join(t.TempDir(), "3")

	testFS := NewFS([]string{
		layer1,
		layer2,
		layer3,
	})

	{
		os.Mkdir(layer1, 0600)
		os.Mkdir(layer2, 0600)
		os.Mkdir(layer3, 0600)

		err := os.WriteFile(path.Join(layer1, "file1"), []byte("1"), 0600)
		assert.NoError(t, err)

		err = os.WriteFile(path.Join(layer2, "file2"), []byte("2"), 0600)
		assert.NoError(t, err)

		err = os.WriteFile(path.Join(layer3, "file3"), []byte("3"), 0600)
		assert.NoError(t, err)
	}

	{
		_, err := testFS.Open("/notexist")
		assert.True(t, errors.Is(err, os.ErrNotExist))
	}

	{
		f, err := testFS.Open("/file1")
		assert.NoError(t, err)
		b, err := io.ReadAll(f)
		assert.NoError(t, err)
		assert.Equal(t, []byte("1"), b)

		f, err = testFS.Open("/file2")
		assert.NoError(t, err)
		b, err = io.ReadAll(f)
		assert.NoError(t, err)
		assert.Equal(t, []byte("2"), b)

		f, err = testFS.Open("file3")
		assert.NoError(t, err)
		b, err = io.ReadAll(f)
		assert.NoError(t, err)
		assert.Equal(t, []byte("3"), b)
	}

	{
		err := os.WriteFile(path.Join(layer1, "file2"), []byte("1"), 0600)
		assert.NoError(t, err)

		f, err := testFS.Open("file2")
		assert.NoError(t, err)
		b, err := io.ReadAll(f)
		assert.NoError(t, err)
		assert.Equal(t, []byte("1"), b)
	}
}

func TestOverlayDir(t *testing.T) {
	layer1 := path.Join(t.TempDir(), "1")
	layer2 := path.Join(t.TempDir(), "2")
	layer3 := path.Join(t.TempDir(), "3")

	testFS := NewFS([]string{
		layer1,
		layer2,
		layer3,
	})

	os.MkdirAll(path.Join(layer1, "dir"), 0700)
	os.MkdirAll(path.Join(layer2, "dir"), 0700)
	os.MkdirAll(path.Join(layer3, "dir"), 0700)

	{
		_, err := testFS.ReadDir("/notexist")
		assert.True(t, errors.Is(err, fs.ErrNotExist))

		err = os.WriteFile(path.Join(layer1, "file1"), []byte("1"), 0600)
		assert.NoError(t, err)

		_, err = testFS.ReadDir("file1")
		assert.True(t, errors.Is(err, syscall.ENOTDIR))

		dir, err := testFS.Open("dir")
		assert.NoError(t, err)

		entries, err := dir.(fs.ReadDirFile).ReadDir(-1)
		assert.NoError(t, err)
		assert.Len(t, entries, 0)
	}

	{

		err := os.WriteFile(path.Join(layer1, "dir", "file1"), []byte("1"), 0600)
		assert.NoError(t, err)

		err = os.WriteFile(path.Join(layer2, "dir", "file2"), []byte("2"), 0600)
		assert.NoError(t, err)

		err = os.WriteFile(path.Join(layer3, "dir", "file3"), []byte("3"), 0600)
		assert.NoError(t, err)

		err = os.Mkdir(path.Join(layer3, "dir", "subdir"), 0700)
		assert.NoError(t, err)

		err = os.WriteFile(path.Join(layer3, "dir", "subdir", "file3"), []byte("3"), 0600)
		assert.NoError(t, err)

		dir, err := testFS.Open("dir")
		assert.NoError(t, err)

		entries, err := dir.(fs.ReadDirFile).ReadDir(-1)
		assert.NoError(t, err)
		assert.Len(t, entries, 4)
		assert.Equal(t, "file1", entries[0].Name())
		assert.Equal(t, "file2", entries[1].Name())
		assert.Equal(t, "file3", entries[2].Name())
		assert.Equal(t, "subdir", entries[3].Name())
		assert.True(t, entries[3].IsDir())

		entries, err = dir.(fs.ReadDirFile).ReadDir(0)
		assert.NoError(t, err)
		assert.Len(t, entries, 4)

		entries, err = dir.(fs.ReadDirFile).ReadDir(2)
		assert.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Equal(t, "file1", entries[0].Name())
		assert.Equal(t, "file2", entries[1].Name())

		entries, err = testFS.ReadDir("dir")
		assert.NoError(t, err)
		assert.Len(t, entries, 4)
		assert.Equal(t, "file1", entries[0].Name())
		assert.Equal(t, "file2", entries[1].Name())
		assert.Equal(t, "file3", entries[2].Name())
		assert.Equal(t, "subdir", entries[3].Name())
		assert.True(t, entries[3].IsDir())

		_, err = testFS.Open(path.Join("dir", "notexist", "file3"))
		assert.Error(t, err)
		assert.True(t, errors.Is(err, fs.ErrNotExist))

		f, err := testFS.Open(path.Join("dir", "subdir", "file3"))
		assert.NoError(t, err)
		b, err := io.ReadAll(f)
		assert.NoError(t, err)
		assert.Equal(t, []byte("3"), b)
		f.Close()

		createWhiteout(t, path.Join(layer2, "dir", "file3"))
		createWhiteout(t, path.Join(layer2, "dir", "file1"))
		assert.NoError(t, err)

		entries, err = testFS.ReadDir("dir")
		assert.NoError(t, err)
		assert.Len(t, entries, 3)
		assert.Equal(t, "file1", entries[0].Name())
		assert.Equal(t, "file2", entries[1].Name())
		assert.Equal(t, "subdir", entries[2].Name())

		createWhiteout(t, path.Join(layer1, "dir", "subdir"))

		entries, err = testFS.ReadDir("dir")
		assert.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Equal(t, "file1", entries[0].Name())
		assert.Equal(t, "file2", entries[1].Name())

		_, err = testFS.ReadDir(path.Join("dir", "subdir"))
		assert.True(t, errors.Is(err, fs.ErrNotExist))

		_, err = testFS.Open(path.Join("dir", "subdir", "notexist"))
		assert.Error(t, err)
		assert.True(t, errors.Is(err, fs.ErrNotExist))

		_, err = testFS.Open(path.Join("dir", "subdir", "file3"))
		assert.Error(t, err)
		assert.True(t, errors.Is(err, fs.ErrNotExist))
	}
}

func TestOverlayDirWhitedout(t *testing.T) {
	layer1 := path.Join(t.TempDir(), "1")
	layer2 := path.Join(t.TempDir(), "2")

	testFS := NewFS([]string{
		layer1,
		layer2,
	})

	os.MkdirAll(path.Join(layer1, "dir"), 0700)
	os.MkdirAll(path.Join(layer2, "dir/ghost/subdir1/subdir2"), 0700)

	err := os.WriteFile(path.Join(layer2, "dir", "file1"), []byte("1"), 0600)
	assert.NoError(t, err)

	err = os.WriteFile(path.Join(layer2, "dir", "ghost", "file2"), []byte("2"), 0600)
	assert.NoError(t, err)

	createWhiteout(t, path.Join(layer1, "dir/ghost"))

	entries, err := testFS.ReadDir("dir")
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, entries[0].Name(), "file1")

	_, err = testFS.Open("dir/ghost/file2")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	_, err = testFS.ReadDir("dir/ghost/subdir1")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	_, err = testFS.ReadDir("dir/ghost/subdir1/subdir2")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func createWhiteout(t *testing.T, path string) {
	err := syscall.Mknod(path, syscall.S_IFCHR, int(whiteoutCharDev))
	assert.NoError(t, err)
}
