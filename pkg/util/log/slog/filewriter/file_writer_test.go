// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filewriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRollingFileWriterSize(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	require.NotNil(t, writer)

	defer writer.Close()

	assert.Equal(t, "test.log", writer.fileName)
	assert.Equal(t, int64(1024), writer.maxFileSize)
	assert.Equal(t, 5, writer.maxRolls)
}

func TestRollingFileWriterWrite(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	data := []byte("test log message\n")
	n, err := writer.Write(data)

	assert.NoError(t, err)
	assert.Equal(t, len(data), n)

	// Verify file was created
	_, err = os.Stat(logPath)
	assert.NoError(t, err)
}

func TestRollingFileWriterMultipleWrites(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	// Write multiple messages
	for i := 0; i < 10; i++ {
		_, err := writer.Write([]byte("test message\n"))
		assert.NoError(t, err)
	}

	// Verify file exists and has content
	stat, err := os.Stat(logPath)
	assert.NoError(t, err)
	assert.Greater(t, stat.Size(), int64(0))
}

func TestRollingFileWriterRollBySize(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// Small max size to trigger rolling
	maxSize := int64(100)
	writer, err := NewRollingFileWriterSize(logPath, maxSize, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	// Write enough data to exceed max size
	data := make([]byte, maxSize+10)
	for i := range data {
		data[i] = 'A'
	}

	_, err = writer.Write(data)
	require.NoError(t, err)

	// Write more to trigger roll
	_, err = writer.Write([]byte("trigger roll\n"))
	require.NoError(t, err)

	// Check that rolled file exists
	rolledPath := filepath.Join(tempDir, "test.log.1")
	_, err = os.Stat(rolledPath)
	require.NoError(t, err)
}

func TestRollingFileWriterClose(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test\n"))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	// Close should be idempotent
	err = writer.Close()
	require.NoError(t, err)
}

func TestRollingFileWriterPostfixNaming(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	assert.Equal(t, RollingNameMode(RollingNameModePostfix), writer.nameMode)

	// Test roll name format
	assert.True(t, writer.hasRollName("test.log.1"))
	assert.True(t, writer.hasRollName("test.log.2"))
	assert.False(t, writer.hasRollName("test.log"))
	assert.False(t, writer.hasRollName("other.log.1"))
}

func TestRollingFileWriterPrefixNaming(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePrefix)
	require.NoError(t, err)
	defer writer.Close()

	assert.Equal(t, RollingNameMode(RollingNameModePrefix), writer.nameMode)

	// Test roll name format
	assert.True(t, writer.hasRollName("1.test.log"))
	assert.True(t, writer.hasRollName("2.test.log"))
	assert.False(t, writer.hasRollName("test.log"))
}

func TestRollingFileWriterIsFileRollNameValid(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	assert.True(t, writer.isFileRollNameValid("1"))
	assert.True(t, writer.isFileRollNameValid("123"))
	assert.False(t, writer.isFileRollNameValid(""))
	assert.False(t, writer.isFileRollNameValid("abc"))
	assert.False(t, writer.isFileRollNameValid("1.2"))
}

func TestRollingFileWriterGetCurrentFileName(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	assert.Equal(t, "test.log", writer.getCurrentFileName())
}

func TestRollingFileWriterString(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	str := writer.String()
	assert.Contains(t, str, "test.log")
	assert.Contains(t, str, "1024")
	assert.Contains(t, str, "5")
}

func TestRollingFileWriterDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "subdir", "nested", "test.log")

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	// Write to trigger directory creation
	_, err = writer.Write([]byte("test\n"))
	assert.NoError(t, err)

	// Verify directory was created
	_, err = os.Stat(filepath.Dir(logPath))
	assert.NoError(t, err)
}

func TestRollingFileWriterMaxRollsZero(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// maxRolls = 0 means unlimited rolls
	writer, err := NewRollingFileWriterSize(logPath, 1024, 0, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	assert.Equal(t, 0, writer.maxRolls)
}

func TestRollingFileWriterRelativePath(t *testing.T) {
	tempDir := t.TempDir()
	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)

	err := os.Chdir(tempDir)
	require.NoError(t, err)

	writer, err := NewRollingFileWriterSize("test.log", 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("test\n"))
	assert.NoError(t, err)

	// Verify file was created in current directory
	_, err = os.Stat("test.log")
	assert.NoError(t, err)
}

func TestGetDirFilePaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files
	testFiles := []string{"file1.txt", "file2.log", "file3.txt"}
	for _, f := range testFiles {
		err := os.WriteFile(filepath.Join(tempDir, f), []byte("test"), 0644)
		require.NoError(t, err)
	}

	// Get all files
	files, err := getDirFilePaths(tempDir, nil, true)
	assert.NoError(t, err)
	assert.Len(t, files, 3)
}

func TestGetDirFilePathsWithFilter(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files
	testFiles := []string{"file1.txt", "file2.log", "file3.txt"}
	for _, f := range testFiles {
		err := os.WriteFile(filepath.Join(tempDir, f), []byte("test"), 0644)
		require.NoError(t, err)
	}

	// Filter for .txt files only
	filter := func(path string) bool {
		return filepath.Ext(path) == ".txt"
	}

	files, err := getDirFilePaths(tempDir, filter, true)
	assert.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestTryRemoveFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Create a test file
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Remove it
	err = tryRemoveFile(testFile)
	assert.NoError(t, err)

	// Verify it's gone
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))

	// Removing non-existent file should not error
	err = tryRemoveFile(testFile)
	assert.NoError(t, err)
}

func TestIsRegular(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	info, err := os.Stat(testFile)
	require.NoError(t, err)

	assert.True(t, isRegular(info.Mode()))

	// Directory should not be regular
	dirInfo, err := os.Stat(tempDir)
	require.NoError(t, err)
	assert.False(t, isRegular(dirInfo.Mode()))
}

func TestRollingFileWriterExceedMaxRolls(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// Set a very small max size to trigger rolling easily
	maxSize := int64(50)
	maxRolls := 3
	writer, err := NewRollingFileWriterSize(logPath, maxSize, maxRolls, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	// Write enough data to trigger multiple rolls
	// Each write is 60 bytes, exceeding maxSize, so each write should trigger a roll
	data := make([]byte, 60)
	for i := range data {
		data[i] = 'A'
	}

	// Write 6 times to create more rolls than maxRolls allows
	// This should create: test.log.1, test.log.2, test.log.3, test.log.4, test.log.5, test.log.6
	// But with maxRolls=3, only the 3 most recent should be kept
	for i := 0; i < 6; i++ {
		_, err = writer.Write(data)
		assert.NoError(t, err)
	}

	// Get all files in directory
	files, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	// Count log files (excluding the current test.log)
	logFiles := make([]string, 0)
	for _, f := range files {
		name := f.Name()
		if name != "test.log" && strings.HasPrefix(name, "test.log.") {
			logFiles = append(logFiles, name)
		}
	}

	// Should have at most maxRolls rolled files (plus the current file)
	assert.LessOrEqual(t, len(logFiles), maxRolls,
		"Expected at most %d rolled files, got %d: %v", maxRolls, len(logFiles), logFiles)

	// Verify the current file exists
	_, err = os.Stat(logPath)
	assert.NoError(t, err)
}

func TestRollingFileWriterExceedMaxRollsPrefix(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// Test with prefix mode
	maxSize := int64(50)
	maxRolls := 2
	writer, err := NewRollingFileWriterSize(logPath, maxSize, maxRolls, RollingNameModePrefix)
	require.NoError(t, err)
	defer writer.Close()

	// Write enough data to trigger multiple rolls
	data := make([]byte, 60)
	for i := range data {
		data[i] = 'B'
	}

	// Write 5 times to exceed maxRolls
	for i := 0; i < 5; i++ {
		_, err = writer.Write(data)
		assert.NoError(t, err)
	}

	// Get all files in directory
	files, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	// Count log files (excluding the current test.log)
	logFiles := make([]string, 0)
	for _, f := range files {
		name := f.Name()
		if name != "test.log" && strings.HasSuffix(name, ".test.log") {
			logFiles = append(logFiles, name)
		}
	}

	// Should have at most maxRolls rolled files
	assert.LessOrEqual(t, len(logFiles), maxRolls,
		"Expected at most %d rolled files, got %d: %v", maxRolls, len(logFiles), logFiles)

	// Verify the current file exists
	_, err = os.Stat(logPath)
	assert.NoError(t, err)
}
