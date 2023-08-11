// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/stretchr/testify/assert"
)

// assertFilesExist verifies that all files in filenames exist in the flare archive
func assertFilesExist(t *testing.T, flare flare.Flare, filenames []string) {
	verifyAssertionsOnFilesList(t, flare, filenames, fileExists)
}

// fileExists verifies if a file exists in the flare archive
func fileExists(t *testing.T, flare flare.Flare, filename string) {
	_, err := flare.GetFile(filename)
	assert.NoError(t, err, "Got error when searching for '%v' file in flare archive: %v", filename, err)
}

// assertFoldersExist verifies that all files in filenames exist in the flare archive and are folders
func assertFoldersExist(t *testing.T, flare flare.Flare, filenames []string) {
	verifyAssertionsOnFilesList(t, flare, filenames, folderExists)
}

// folderExists verifies if a file exists in the flare archive and is a folder
func folderExists(t *testing.T, flare flare.Flare, filename string) {
	fileInfo, err := flare.GetFileInfo(filename)
	assert.NoError(t, err, "Got error when searching for '%v' file in flare archive: %v", filename, err)

	if err == nil {
		assert.True(t, fileInfo.IsDir(), "Expected '%v' to be a folder but is not. (FileMode: %v)", filename, fileInfo.Mode())
	}
}

// assertLogsFolderOnlyContainsLogFile verifies that all files in "logs" folder are logs file (filename containing ".log") or folders
func assertLogsFolderOnlyContainsLogFile(t *testing.T, flare flare.Flare) {
	// Get all files in "logs/" folder
	logFiles := filterFilenameByPrefix(flare.GetFilenames(), "logs/")
	verifyAssertionsOnFilesList(t, flare, logFiles, assertIsLogFileOrFolder)
}

// assertIsLogFileOrFolder verifies if a file is a log file (contains ".log" in its name) or if it's a folder
func assertIsLogFileOrFolder(t *testing.T, flare flare.Flare, filename string) {
	isLogFileOrFolder := strings.Contains(filename, ".log") || isDir(flare, filename)
	assert.True(t, isLogFileOrFolder, "'%v' is in logs/ folder but is not a log file (does not contains .log, and is not a folder)", filename)
}

// assertLogsFolderOnlyContainsLogFile verifies that all files in "etc" folder are configuration file (filename containing ".yaml" / ".yml") or folders
func assertEtcFolderOnlyContainsConfigFile(t *testing.T, flare flare.Flare) {
	// Get all files in "etc/" folder
	configFiles := filterFilenameByPrefix(flare.GetFilenames(), "etc/")
	verifyAssertionsOnFilesList(t, flare, configFiles, assertIsConfigFileOrFolder)
}

// assertIsConfigFileOrFolder verifies if a file is a configuration file (contains ".yaml"/".yml" in its name) or if it's a folder
func assertIsConfigFileOrFolder(t *testing.T, flare flare.Flare, filename string) {
	assert.False(t, strings.HasSuffix(filename, ".example"), "Found '%v' configuration file but example configurations should not be included in flare")

	isConfigFileOrFolder := strings.Contains(filename, ".yml") || strings.Contains(filename, ".yaml") || isDir(flare, filename)
	assert.True(t, isConfigFileOrFolder, "'%v' is in etc/ folder but is not a configuration file (does not contains .yml or .yaml, and is not a folder)", filename)
}

// verifyAssetionsOnFilesList runs an assertion function on all files in filenames
func verifyAssertionsOnFilesList(t *testing.T, flare flare.Flare, filenames []string, assertFn func(*testing.T, flare.Flare, string)) {
	for _, filename := range filenames {
		assertFn(t, flare, filename)
	}
}

// filterFilenameByPrefix returns all filenames starting with `suffix`.
// This is used to get all files from a folder since all files in the 'foo' folder have a name starting with "foo/"
func filterFilenameByPrefix(filenames []string, suffix string) []string {
	var filteredFilenames []string

	for _, filename := range filenames {
		if strings.HasPrefix(filename, suffix) {
			filteredFilenames = append(filteredFilenames, filename)
		}
	}
	return filteredFilenames
}

// isDir returns true if a `filename` is a directory
func isDir(flare flare.Flare, filename string) bool {
	fileInfo, err := flare.GetFileInfo(filename)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func assertProcessCheckShouldBeEnabled(t *testing.T, flare flare.Flare, checkName string, setting string, shouldBeEnabled bool) {
	filename := fmt.Sprintf("%s_check_output.json", checkName)
	expectedContentIfDisabled := fmt.Sprintf("'%s' is disabled", setting)

	if shouldBeEnabled {
		assertFileNotContains(t, flare, filename, []string{expectedContentIfDisabled})
	} else {
		assertFileContains(t, flare, filename, []string{expectedContentIfDisabled})
	}
}

func assertFileContains(t *testing.T, flare flare.Flare, filename string, expectedContents []string) {
	fileContent, err := flare.GetFileContent(filename)
	if assert.NoError(t, err) {
		for _, content := range expectedContents {
			assert.Contains(t, fileContent, content)
		}
	}
}

func assertFileNotContains(t *testing.T, flare flare.Flare, filename string, expectedContents []string) {
	fileContent, err := flare.GetFileContent(filename)
	if assert.NoError(t, err) {
		for _, content := range expectedContents {
			assert.NotContains(t, fileContent, content)
		}
	}
}
