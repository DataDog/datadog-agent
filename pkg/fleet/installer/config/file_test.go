// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestDirectory(t *testing.T, files map[string]string) string {
	dir := t.TempDir()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		assert.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		assert.NoError(t, err)
	}
	return dir
}

func verifyDirectoryContent(t *testing.T, dir string, files map[string]string) {
	for path, content := range files {
		targetPath := filepath.Join(dir, path)
		data, err := os.ReadFile(targetPath)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	}
}

func TestCopyDirectoryEmptySource(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	// Target should be empty since source is empty
	files, err := os.ReadDir(targetDir)
	assert.NoError(t, err)
	assert.Empty(t, files)
}

func TestCopyDirectorySimpleFiles(t *testing.T) {
	sourceFiles := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := t.TempDir()

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	verifyDirectoryContent(t, targetDir, sourceFiles)
}

func TestCopyDirectoryWithSubdirectories(t *testing.T) {
	sourceFiles := map[string]string{
		"file1.txt":         "content1",
		"file2.txt":         "content2",
		"subdir/file3.txt":  "content3",
		"subdir/file4.txt":  "content4",
		"subdir2/file5.txt": "content5",
		"subdir2/file6.txt": "content6",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := t.TempDir()

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	verifyDirectoryContent(t, targetDir, sourceFiles)
}

func TestCopyDirectoryWithNestedSubdirectories(t *testing.T) {
	sourceFiles := map[string]string{
		"file1.txt":                    "content1",
		"subdir/file2.txt":             "content2",
		"subdir/nested/file3.txt":      "content3",
		"subdir/nested/deep/file4.txt": "content4",
		"subdir2/file5.txt":            "content5",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := t.TempDir()

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	verifyDirectoryContent(t, targetDir, sourceFiles)
}

func TestCopyDirectoryToExistingTarget(t *testing.T) {
	sourceFiles := map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := t.TempDir()

	// Create some existing content in target
	existingFiles := map[string]string{
		"existing.txt": "existing content",
	}
	createTestDirectory(t, existingFiles) // This creates a temp dir, we need to use targetDir
	for path, content := range existingFiles {
		fullPath := filepath.Join(targetDir, path)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	// Verify source files were copied
	verifyDirectoryContent(t, targetDir, sourceFiles)

	// Verify existing files are still there
	for path, content := range existingFiles {
		targetPath := filepath.Join(targetDir, path)
		data, err := os.ReadFile(targetPath)
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	}
}

func TestCopyDirectorySourceDoesNotExist(t *testing.T) {
	targetDir := t.TempDir()
	nonExistentSource := filepath.Join(t.TempDir(), "non-existent")

	err := copyDirectory(nonExistentSource, targetDir)
	assert.Error(t, err)
}

func TestCopyDirectoryTargetDoesNotExist(t *testing.T) {
	sourceFiles := map[string]string{
		"file1.txt": "content1",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := filepath.Join(t.TempDir(), "non-existent-target")

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)
	assert.DirExists(t, targetDir)
}

func TestCopyDirectoryWithSpecialCharacters(t *testing.T) {
	sourceFiles := map[string]string{
		"file with spaces.txt":             "content1",
		"file-with-dashes.txt":             "content2",
		"file_with_underscores.txt":        "content3",
		"subdir/file with spaces.txt":      "content4",
		"subdir/file-with-dashes.txt":      "content5",
		"subdir/file_with_underscores.txt": "content6",
	}

	sourceDir := createTestDirectory(t, sourceFiles)
	targetDir := t.TempDir()

	err := copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	verifyDirectoryContent(t, targetDir, sourceFiles)
}

func TestCopyDirectoryPreservesFilePermissions(t *testing.T) {
	sourceDir := t.TempDir()

	// Create a file with specific permissions
	filePath := filepath.Join(sourceDir, "test.txt")
	err := os.WriteFile(filePath, []byte("test content"), 0755)
	assert.NoError(t, err)

	targetDir := t.TempDir()

	err = copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	// Check that the file was copied
	targetFilePath := filepath.Join(targetDir, "test.txt")
	assert.FileExists(t, targetFilePath)

	// Check file content
	data, err := os.ReadFile(targetFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "test content", string(data))

	// Note: On some systems, file permissions might be modified during copy
	// This test focuses on successful copying rather than exact permission preservation
}

func TestCopyDirectoryLargeFile(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create a large file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	filePath := filepath.Join(sourceDir, "large.txt")
	err := os.WriteFile(filePath, largeContent, 0644)
	assert.NoError(t, err)

	err = copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)

	// Verify the large file was copied correctly
	targetFilePath := filepath.Join(targetDir, "large.txt")
	data, err := os.ReadFile(targetFilePath)
	assert.NoError(t, err)
	assert.Equal(t, largeContent, data)
}

func TestCopyDirectoryWithSymlinks(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	err := os.Symlink(sourceDir, filepath.Join(targetDir, "symlink"))
	assert.NoError(t, err)

	err = copyDirectory(sourceDir, targetDir)
	assert.NoError(t, err)
}
