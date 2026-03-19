// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTarFile adds a regular file entry to a tar writer.
func writeTarFile(tw *tar.Writer, name string, content []byte) {
	_ = tw.WriteHeader(&tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(content)
}

// writeTarDir adds a directory entry to a tar writer.
func writeTarDir(tw *tar.Writer, name string) {
	_ = tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	})
}

func TestExtractTar_SingleFile(t *testing.T) {
	// Simulate: tar cf - -C /tmp myfile.zip
	// The archive contains a single file entry named "myfile.zip".
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarFile(tw, "myfile.zip", []byte("file-contents"))
	tw.Close()

	destPath := t.TempDir()
	err := extractTar(&buf, "myfile.zip", destPath)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destPath, "myfile.zip"))
	require.NoError(t, err)
	assert.Equal(t, "file-contents", string(content))
}

func TestExtractTar_Directory(t *testing.T) {
	// Simulate: tar cf - -C /tmp mydir
	// The archive contains:
	//   mydir/
	//   mydir/a.txt
	//   mydir/sub/
	//   mydir/sub/b.txt
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarDir(tw, "mydir/")
	writeTarFile(tw, "mydir/a.txt", []byte("aaa"))
	writeTarDir(tw, "mydir/sub/")
	writeTarFile(tw, "mydir/sub/b.txt", []byte("bbb"))
	tw.Close()

	destPath := t.TempDir()
	err := extractTar(&buf, "mydir", destPath)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "aaa", string(content))

	content, err = os.ReadFile(filepath.Join(destPath, "sub", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bbb", string(content))

	// The top-level "mydir" directory should not appear in destPath
	_, err = os.Stat(filepath.Join(destPath, "mydir"))
	assert.True(t, os.IsNotExist(err))
}

func TestExtractTar_DirectoryWithoutTrailingSlash(t *testing.T) {
	// Some tar implementations emit directory headers without a trailing slash.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarDir(tw, "mydir")
	writeTarFile(tw, "mydir/a.txt", []byte("aaa"))
	tw.Close()

	destPath := t.TempDir()
	err := extractTar(&buf, "mydir", destPath)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "aaa", string(content))

	_, err = os.Stat(filepath.Join(destPath, "mydir"))
	assert.True(t, os.IsNotExist(err))
}

