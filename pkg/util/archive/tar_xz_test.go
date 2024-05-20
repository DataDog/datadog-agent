// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package archive

import (
	"archive/tar"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const archive = "./testdata/tartest.tar.xz"

func TestTarXZExtractFile(t *testing.T) {
	err := TarXZExtractFile(archive, "notfound", t.TempDir())
	assert.Error(t, err, "file not in archive should be not found")

	tmp := t.TempDir()
	err = TarXZExtractFile(archive, "testfile", tmp)
	if assert.NoError(t, err) {
		testpath := filepath.Join(tmp, "testfile")
		if assert.FileExists(t, testpath) {
			fi, err := os.Stat(testpath)
			if assert.NoError(t, err) {
				if runtime.GOOS == "windows" {
					// windows doesn't really have separate owner/group/world perms
					assert.Equal(t, fs.FileMode(0444), fi.Mode())
				} else {
					assert.Equal(t, fs.FileMode(0400), fi.Mode())
				}
			}
		}
	}

	tmp = t.TempDir()
	err = TarXZExtractFile(archive, "nested/testfile", tmp)
	if assert.NoError(t, err) {
		testpath := filepath.Join(tmp, "nested/testfile")
		assert.FileExists(t, testpath)
	}
}

func TestTarXZExtractAll(t *testing.T) {
	tmp := t.TempDir()
	err := TarXZExtractAll(archive, tmp)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(tmp, "testfile"))
	require.FileExists(t, filepath.Join(tmp, "nested/testfile"))
}

func TestWalkTarXZArchive(t *testing.T) {
	var foundpaths []string
	err := WalkTarXZArchive(archive, func(rdr *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeReg {
			foundpaths = append(foundpaths, hdr.Name)
		}
		return nil
	})
	if assert.NoError(t, err) {
		assert.ElementsMatch(t, foundpaths, []string{"testfile", "nested/testfile"})
	}

	foundpaths = []string{}
	err = WalkTarXZArchive(archive, func(rdr *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeReg {
			foundpaths = append(foundpaths, hdr.Name)
			return ErrStopWalk
		}
		return nil
	})
	if assert.NoError(t, err) {
		assert.ElementsMatch(t, foundpaths, []string{"nested/testfile"})
	}
}
