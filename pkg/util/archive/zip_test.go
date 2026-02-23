// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestZip_WrongDestinationFile(t *testing.T) {
	err := Zip([]string{"a"}, "destination.txt")

	assert.Error(t, err, "must have a .zip extension")
}

func TestZip_ExistingDestination(t *testing.T) {
	tmpDir := t.TempDir()
	tempLocation := filepath.Join(tmpDir, "destination.zip")
	tempLocationFileDescriptor, err := os.Create(tempLocation)
	assert.Nil(t, err)

	defer tempLocationFileDescriptor.Close()

	err = Zip([]string{"a"}, tempLocation)

	assert.Error(t, err, "file already exists:")
}

func TestZip_DoNotZipSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows. Symlinks are not supported.")
	}

	tmpDestinationDir := t.TempDir()
	zipTempLocation := filepath.Join(tmpDestinationDir, "destination.zip")

	tmpDir := t.TempDir()

	// Create a regular file and a symlink
	target := filepath.Join(tmpDir, "symtarget.txt")
	err := os.WriteFile(target, []byte("Hello\n"), 0755)
	assert.Nil(t, err)
	symlink := filepath.Join(tmpDir, "symlink")
	err = os.Symlink(target, symlink)
	assert.Nil(t, err)

	// Create nested directory
	nestedDirectory := filepath.Join(tmpDir, "nested")
	err = os.MkdirAll(nestedDirectory, 0755)
	assert.Nil(t, err)
	err = os.WriteFile(filepath.Join(nestedDirectory, "nested_file.txt"), []byte("Hello\n"), 0755)
	assert.Nil(t, err)

	err = Zip([]string{target, symlink, nestedDirectory}, zipTempLocation)

	assert.Nil(t, err)

	// Unzip the archive
	destinmationDir := t.TempDir()
	err = Unzip(zipTempLocation, destinmationDir)
	assert.Nil(t, err)

	// 'symtarget.txt' safely extracted without errors inside the destination path
	_, err = os.Stat(filepath.Join(destinmationDir, "symtarget.txt"))
	assert.Nil(t, err, "symtarget.txt should be extracted inside the destination folder")

	// 'symlink' is not zipped
	_, err = os.Stat(filepath.Join(destinmationDir, "symlink"))
	assert.True(t, os.IsNotExist(err))

	// 'nested/nested_file.txt' is not a symlink and should be extracted without errors inside the destination path
	_, err = os.Stat(filepath.Join(tmpDir, "nested", "nested_file.txt"))
	assert.Nil(t, err, "nested/nested_file.txt should be extracted inside the destination folder")
}

func TestUnzip(t *testing.T) {
	destinationZip := createUnsafeZip(t, false)

	tmpDir := t.TempDir()

	err := Unzip(destinationZip, tmpDir)

	assert.Nil(t, err)

	// 'goodfile.txt' safely extracted without errors inside the destination path
	_, err = os.Stat(filepath.Join(tmpDir, "goodfile.txt"))
	assert.Nil(t, err, "goodfile.txt should be extracted inside the destination folder")

	// 'bad/file.txt' is not a symlink and should be extracted without errors inside the destination path
	fileInfo, err := os.Stat(filepath.Join(tmpDir, "bad", "file.txt"))
	assert.Nil(t, err, "bad/file.txt should be extracted inside the destination folder")
	assert.True(t, fileInfo.Mode() != os.ModeSymlink)
}

func TestUnzip_OutsideRoot(t *testing.T) {
	destinationZip := createUnsafeZip(t, true)

	tmpDir := t.TempDir()

	err := Unzip(destinationZip, tmpDir)

	assert.Nil(t, err)

	// '../../../../../badfile.txt' should be extracted inside the destination folder
	_, err = os.Stat(filepath.Join(tmpDir, "badfile.txt"))
	assert.Nil(t, err, "badfile.txt should be extracted inside the destination folder")
}

type file struct {
	Name, Body string
}

func TestZipAndUnzipRoundTrip(t *testing.T) {
	srcDir := t.TempDir()

	// Create files to zip
	err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)
	assert.NoError(t, err)

	destDir := t.TempDir()
	zipPath := filepath.Join(destDir, "test.zip")

	err = Zip([]string{srcDir}, zipPath)
	assert.NoError(t, err)

	unzipDir := t.TempDir()
	err = Unzip(zipPath, unzipDir)
	assert.NoError(t, err)

	// Verify files were extracted
	content1, err := os.ReadFile(filepath.Join(unzipDir, filepath.Base(srcDir), "file1.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "content1", string(content1))

	content2, err := os.ReadFile(filepath.Join(unzipDir, filepath.Base(srcDir), "subdir", "file2.txt"))
	assert.NoError(t, err)
	assert.Equal(t, "content2", string(content2))
}

func TestZip_NonExistentSource(t *testing.T) {
	destDir := t.TempDir()
	zipPath := filepath.Join(destDir, "test.zip")

	err := Zip([]string{"/nonexistent/path"}, zipPath)
	assert.Error(t, err)
}

func TestZip_CreatesDestinationDir(t *testing.T) {
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	assert.NoError(t, err)

	destDir := filepath.Join(t.TempDir(), "newdir")
	zipPath := filepath.Join(destDir, "test.zip")

	err = Zip([]string{srcDir}, zipPath)
	assert.NoError(t, err)
	assert.True(t, fileExists(zipPath))
}

func TestUnzip_NonExistentSource(t *testing.T) {
	err := Unzip("/nonexistent/path.zip", t.TempDir())
	assert.Error(t, err)
}

func TestUnzip_CreatesDestinationDir(t *testing.T) {
	// Create a valid zip first
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	assert.NoError(t, err)

	zipDir := t.TempDir()
	zipPath := filepath.Join(zipDir, "test.zip")
	err = Zip([]string{filepath.Join(srcDir, "file.txt")}, zipPath)
	assert.NoError(t, err)

	// Unzip into a non-existent directory
	destDir := filepath.Join(t.TempDir(), "newdir", "subdir")
	err = Unzip(zipPath, destDir)
	assert.NoError(t, err)
}

func TestWithin(t *testing.T) {
	assert.True(t, within("/a/b", "/a/b/c"))
	assert.True(t, within("/a/b", "/a/b"))
	assert.False(t, within("/a/b/c", "/a/b"))
	assert.False(t, within("/a/b", "/a/c"))
}

func TestMakeNameInArchive(t *testing.T) {
	t.Run("file source", func(t *testing.T) {
		info, err := os.Stat(os.Args[0]) // use any existing file
		if err != nil {
			t.Skip("cannot stat test binary")
		}
		// For a non-directory source, it just uses the base name
		name, err := makeNameInArchive(info, "/some/path/file.txt", "", "/some/path/file.txt")
		assert.NoError(t, err)
		assert.Equal(t, "file.txt", name)
	})

	t.Run("with base dir", func(t *testing.T) {
		info, err := os.Stat(os.Args[0])
		if err != nil {
			t.Skip("cannot stat test binary")
		}
		name, err := makeNameInArchive(info, "/some/path/file.txt", "base", "/some/path/file.txt")
		assert.NoError(t, err)
		assert.Equal(t, "base/file.txt", name)
	})
}

func TestFileInfoName(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	err := os.WriteFile(tmpFile, []byte("test"), 0644)
	assert.NoError(t, err)

	info, err := os.Stat(tmpFile)
	assert.NoError(t, err)

	fi := fileInfo{FileInfo: info, customName: "custom.txt"}
	assert.Equal(t, "custom.txt", fi.Name())

	fi2 := fileInfo{FileInfo: info}
	assert.Equal(t, "test.txt", fi2.Name())
}

func createUnsafeZip(t *testing.T, createFileOutsideRoot bool) string {
	// Create a buffer to write our archive to.
	tmpDir := t.TempDir()

	path := filepath.Join(tmpDir, "unsafe.zip")
	fw, err := os.Create(path)
	if nil != err {
		t.Fatalf("Failed to create zip file: %s", err)
	}

	// Create a new zip archive.
	w := zip.NewWriter(fw)

	// Write the unsafe symlink
	h := &zip.FileHeader{
		Name:     "bad/file.txt",
		Method:   zip.Deflate,
		Modified: time.Now(),
	}
	h.SetMode(os.ModeSymlink)
	header, err := w.CreateHeader(h)
	if err != nil {
		t.Fatalf("Failed to create file header: %s", err)
	}
	// The unsafe symlink points outside of the target directory
	_, err = header.Write([]byte("../../badfile.txt"))
	if err != nil {
		t.Fatalf("Failed to write file: %s", err)
	}

	// Write safe files to the archive.
	var files = []file{
		{"goodfile.txt", "hello world"},
		{"morefile.txt", "hello world"},
		{"bad/file.txt", "Mwa-ha-ha"},
	}

	if createFileOutsideRoot {
		files = append(files, file{"../../../../../badfile.txt", "outside of root"})
	}

	for _, file := range files {
		h := &zip.FileHeader{
			Name:     file.Name,
			Method:   zip.Deflate,
			Modified: time.Now(),
		}

		header, err := w.CreateHeader(h)
		if err != nil {
			t.Fatalf("Failed to create file header: %s", err)
		}

		_, err = header.Write([]byte(file.Body))
		if err != nil {
			t.Fatalf("Failed to write file: %s", err)
		}
	}

	// close the in-memory archive so that it writes trailing data
	if err = w.Close(); err != nil {
		t.Fatalf("Failed to close file: %s", err)
	}

	// close the on-disk archive so that it flushes all bytes
	if err = fw.Close(); err != nil {
		t.Fatalf("Failed to close file: %s", err)
	}

	return path
}
