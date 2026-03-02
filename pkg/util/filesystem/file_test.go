// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	// existing file
	f := filepath.Join(dir, "exists.txt")
	require.NoError(t, os.WriteFile(f, []byte("hi"), 0644))
	assert.True(t, FileExists(f))

	// non-existing file
	assert.False(t, FileExists(filepath.Join(dir, "nope.txt")))
}

func TestReadLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	require.NoError(t, os.WriteFile(f, []byte("line1\nline2\nline3"), 0644))

	lines, err := ReadLines(f)
	require.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)

	// non-existing file returns error
	_, err = ReadLines(filepath.Join(dir, "nope.txt"))
	assert.Error(t, err)
}

func TestGetFileSize(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sized.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(f, content, 0644))

	size, err := GetFileSize(f)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), size)

	// non-existing file
	_, err = GetFileSize(filepath.Join(dir, "nope.txt"))
	assert.Error(t, err)
}

func TestGetFileModTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "timed.txt")
	before := time.Now().Add(-time.Second)
	require.NoError(t, os.WriteFile(f, []byte("data"), 0644))

	modTime, err := GetFileModTime(f)
	require.NoError(t, err)
	assert.True(t, modTime.After(before))

	// non-existing file
	_, err = GetFileModTime(filepath.Join(dir, "nope.txt"))
	assert.Error(t, err)
}

func TestEnsureParentDirsExist(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "file.txt")

	require.NoError(t, EnsureParentDirsExist(nested))
	assert.True(t, FileExists(filepath.Join(dir, "a", "b", "c")))
}

func TestCopyFileAll(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("content"), 0644))

	dst := filepath.Join(dir, "sub", "dir", "dst.txt")
	require.NoError(t, CopyFileAll(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "content", string(data))
}

func TestCopyDir(t *testing.T) {
	assert := assert.New(t)
	src := t.TempDir()
	dst := t.TempDir()

	files := map[string]string{
		"a/b/c/d.txt": "d.txt",
		"e/f/g/h.txt": "h.txt",
		"i/j/k.txt":   "k.txt",
	}

	for file, content := range files {
		p := filepath.Join(src, file)
		err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
		assert.NoError(err)
		err = os.WriteFile(p, []byte(content), os.ModePerm)
		assert.NoError(err)
	}
	err := CopyDir(src, dst)
	assert.NoError(err)

	for file, content := range files {
		p := filepath.Join(dst, file)
		actual, err := os.ReadFile(p)
		assert.NoError(err)
		assert.Equal(string(actual), content)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()

	// Test successful copy
	src := filepath.Join(dir, "src.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(src, content, 0644))

	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, CopyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, data)

	// Verify permissions are preserved
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())

	// Test copy of non-existent source
	err = CopyFile(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "out.txt"))
	assert.Error(t, err)
}

func TestCopyFilePreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "executable.sh")
	require.NoError(t, os.WriteFile(src, []byte("#!/bin/bash"), 0755))

	dst := filepath.Join(dir, "executable_copy.sh")
	require.NoError(t, CopyFile(src, dst))

	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
}

func TestCopyDirErrors(t *testing.T) {
	// Test copying non-existent directory
	err := CopyDir("/nonexistent/path", t.TempDir())
	assert.Error(t, err)

	// Test copying to invalid destination
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("data"), 0644))

	// Create a file where the destination should be a directory
	dstParent := t.TempDir()
	dstFile := filepath.Join(dstParent, "dst")
	require.NoError(t, os.WriteFile(dstFile, []byte("blocking"), 0644))
	require.NoError(t, os.Chmod(dstFile, 0000))
	defer os.Chmod(dstFile, 0644) // cleanup

	err = CopyDir(src, filepath.Join(dstFile, "subdir"))
	assert.Error(t, err)
}

func TestOpenFileForWriting(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "output.txt")

	f, writer, err := OpenFileForWriting(filePath)
	require.NoError(t, err)
	require.NotNil(t, f)
	require.NotNil(t, writer)

	// Write some data
	_, err = writer.WriteString("test content")
	require.NoError(t, err)
	require.NoError(t, writer.Flush())
	require.NoError(t, f.Close())

	// Verify content
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))

	// Test that it truncates existing file
	f2, writer2, err := OpenFileForWriting(filePath)
	require.NoError(t, err)
	_, err = writer2.WriteString("new")
	require.NoError(t, err)
	require.NoError(t, writer2.Flush())
	require.NoError(t, f2.Close())

	data, err = os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
}

func TestOpenFileForWritingError(t *testing.T) {
	// Try to open file in non-existent directory
	_, _, err := OpenFileForWriting("/nonexistent/directory/file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error opening file")
}

func TestReadLinesEmpty(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(f, []byte(""), 0644))

	lines, err := ReadLines(f)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

func TestReadLinesSingleLine(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "single.txt")
	require.NoError(t, os.WriteFile(f, []byte("single line"), 0644))

	lines, err := ReadLines(f)
	require.NoError(t, err)
	assert.Equal(t, []string{"single line"}, lines)
}
