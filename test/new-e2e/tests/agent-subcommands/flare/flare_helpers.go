// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/stretchr/testify/assert"
)

// AssertFilesExist verifies that all files in filenames exist in the flare archive
func AssertFilesExist(t *testing.T, f flare.Flare, filenames []string) {
	t.Helper()

	verifyAssertionsOnFilesList(t, f, filenames, fileExists)
}

// fileExists verifies if a file exists in the flare archive
func fileExists(t *testing.T, f flare.Flare, filename string) {
	t.Helper()

	fileInfo, err := f.GetFileInfo(filename)
	if assert.NoError(t, err, "Got error when searching for '%v' file in flare archive: %v", filename, err) {
		assert.True(t, fileInfo.Mode().IsRegular(), "Expected '%v' to be a regular file but is not. (FileMode: %v)", filename, fileInfo.Mode())
	}
}

// AssertFoldersExist verifies that all files in filenames exist in the flare archive and are folders
func AssertFoldersExist(t *testing.T, f flare.Flare, filenames []string) {
	t.Helper()

	verifyAssertionsOnFilesList(t, f, filenames, folderExists)
}

// folderExists verifies if a file exists in the flare archive and is a folder
func folderExists(t *testing.T, f flare.Flare, filename string) {
	t.Helper()

	fileInfo, err := f.GetFileInfo(filename)
	if assert.NoError(t, err, "Got error when searching for '%v' file in flare archive: %v", filename, err) {
		assert.True(t, fileInfo.IsDir(), "Expected '%v' to be a folder but is not. (FileMode: %v)", filename, fileInfo.Mode())
	}
}

// AssertLogsFolderOnlyContainsLogFile verifies that all files in "logs" folder are logs file (filename containing ".log") or folders
func AssertLogsFolderOnlyContainsLogFile(t *testing.T, f flare.Flare) {
	t.Helper()

	// Get all files in "logs/" folder
	logFiles := filterFilenameByPrefix(f.GetFilenames(), "logs/")
	verifyAssertionsOnFilesList(t, f, logFiles, assertIsLogFileOrFolder)
}

// assertIsLogFileOrFolder verifies if a file is a log file (contains ".log" in its name) or if it's a folder
func assertIsLogFileOrFolder(t *testing.T, f flare.Flare, filename string) {
	t.Helper()

	isLogFileOrFolder := strings.Contains(filename, ".log") || isDir(f, filename)
	assert.True(t, isLogFileOrFolder, "'%v' is in logs/ folder but is not a log file (does not contains .log, and is not a folder)", filename)
}

// AssertEtcFolderOnlyContainsConfigFile verifies that all files in "etc" folder are configuration file (filename containing ".yaml" / ".yml") or folders
func AssertEtcFolderOnlyContainsConfigFile(t *testing.T, f flare.Flare) {
	t.Helper()

	// Get all files in "etc/" folder
	configFiles := filterFilenameByPrefix(f.GetFilenames(), "etc/")
	verifyAssertionsOnFilesList(t, f, configFiles, assertIsConfigFileOrFolder)
}

// assertIsConfigFileOrFolder verifies if a file is a configuration file (contains ".yaml"/".yml" in its name) or if it's a folder
func assertIsConfigFileOrFolder(t *testing.T, f flare.Flare, filename string) {
	t.Helper()

	assert.False(t, strings.HasSuffix(filename, ".example"), "Found '%v' configuration file but example configurations should not be included in flare")

	isConfigFileOrFolder := strings.Contains(filename, ".yml") || strings.Contains(filename, ".yaml") || isDir(f, filename)
	assert.True(t, isConfigFileOrFolder, "'%v' is in etc/ folder but is not a configuration file (does not contains .yml or .yaml, and is not a folder)", filename)
}

// AssertEventlogFolderOnlyContainsWindowsEventLog verifies that all files in "eventlog" (windows) folder are Windows Event log file (name ends with .evtx) or folders
func AssertEventlogFolderOnlyContainsWindowsEventLog(t *testing.T, f flare.Flare) {
	t.Helper()

	// Get all files in "eventlog/" folder
	configFiles := filterFilenameByPrefix(f.GetFilenames(), "eventlog/")
	verifyAssertionsOnFilesList(t, f, configFiles, assertIsWindowsEventLogOrFolder)
}

// assertIsWindowsEventLogOrFolder verifies if a file is a Windows Event log file (name ends with .evtx) or if it's a folder
func assertIsWindowsEventLogOrFolder(t *testing.T, f flare.Flare, filename string) {
	t.Helper()

	isWindowsEventLogFolder := strings.Contains(filename, ".evtx") || isDir(f, filename)
	assert.True(t, isWindowsEventLogFolder, "'%v' is in eventlog/ folder but is not a Windows Event Log file (extension is not .evtx, and is not a folder)", filename)
}

// verifyAssertionsOnFilesList runs an assertion function on all files in filenames
func verifyAssertionsOnFilesList(t *testing.T, f flare.Flare, filenames []string, assertFn func(*testing.T, flare.Flare, string)) {
	t.Helper()

	for _, filename := range filenames {
		assertFn(t, f, filename)
	}
}

// filterFilenameByPrefix returns all filenames starting with `suffix`.
// This is used to get all files from a folder since all files in the 'foo' folder have a name starting with "foo/"
func filterFilenameByPrefix(filenames []string, prefix string) []string {
	var filteredFilenames []string

	for _, filename := range filenames {
		if strings.HasPrefix(filename, prefix) {
			filteredFilenames = append(filteredFilenames, filename)
		}
	}
	return filteredFilenames
}

// isDir returns true if a `filename` is a directory
func isDir(f flare.Flare, filename string) bool {
	fileInfo, err := f.GetFileInfo(filename)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// AssertFileContains verifies that `filename` contains every string in `expectedContents`
func AssertFileContains(t *testing.T, f flare.Flare, filename string, expectedContents ...string) {
	t.Helper()

	fileContent, err := f.GetFileContent(filename)
	if assert.NoError(t, err) {
		for _, content := range expectedContents {
			assert.Contains(t, fileContent, content)
		}
	}
}

// AssertFileNotContains verifies that `filename` does not contain any string in `expectedContents`
func AssertFileNotContains(t *testing.T, f flare.Flare, filename string, expectedContents ...string) {
	t.Helper()

	fileContent, err := f.GetFileContent(filename)
	if assert.NoError(t, err) {
		for _, content := range expectedContents {
			assert.NotContains(t, fileContent, content)
		}
	}
}
