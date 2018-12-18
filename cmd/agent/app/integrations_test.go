// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package app

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMoveConfigurationsFiles(t *testing.T) {
	srcFolder, _ := ioutil.TempDir("", "srcFolder")
	dstFolder, _ := ioutil.TempDir("", "dstFolder")
	defer os.RemoveAll(srcFolder)
	defer os.RemoveAll(dstFolder)
	yamlFiles := []string{"conf.yaml.example", "conf.yaml.default", "metrics.yaml", "auto_conf.yaml"}
	otherFile := "not_moved.txt"
	for _, filename := range append(yamlFiles, otherFile) {
		os.Create(filepath.Join(srcFolder, filename))
	}

	filesCreated, _ := ioutil.ReadDir(srcFolder)
	assert.Equal(t, 5, len(filesCreated))
	for _, file := range filesCreated {
		assert.Contains(t, append(yamlFiles, otherFile), file.Name())
	}

	moveConfigurationFiles(srcFolder, dstFolder)

	filesMoved, _ := ioutil.ReadDir(dstFolder)
	assert.Equal(t, 4, len(filesMoved))
	for _, file := range filesMoved {
		assert.Contains(t, yamlFiles, file.Name())
	}
	filesNotMoved, _ := ioutil.ReadDir(srcFolder)
	assert.Equal(t, 1, len(filesNotMoved))
	assert.Equal(t, otherFile, filesNotMoved[0].Name())
}

func TestParseVersion(t *testing.T) {
	version, err := parseVersion("1.2.3")
	assert.Nil(t, err)
	assert.Equal(t, 1, version.major)
	assert.Equal(t, 2, version.minor)
	assert.Equal(t, 3, version.fix)

	version, err = parseVersion("1.2")
	assert.Nil(t, version)
	assert.NotNil(t, err)

	version, err = parseVersion("")
	assert.Nil(t, version)
	assert.Nil(t, err)
}

func TestIsAboveOrEqualTo(t *testing.T) {
	baseVersion, _ := parseVersion("1.2.3")

	version, _ := parseVersion("1.2.3")
	assert.True(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("1.2.4")
	assert.True(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("1.3.0")
	assert.True(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("2.0.0")
	assert.True(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("1.1.9")
	assert.False(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("0.0.1")
	assert.False(t, version.isAboveOrEqualTo(baseVersion))

	version, _ = parseVersion("1.2.2")
	assert.False(t, version.isAboveOrEqualTo(baseVersion))

	baseVersion = nil
	assert.True(t, version.isAboveOrEqualTo(baseVersion))
}

func TestEquals(t *testing.T) {
	v1, _ := parseVersion("1.2.3")
	v2, _ := parseVersion("1.2.3")

	assert.True(t, v1.equals(v2))

	v2 = nil
	assert.False(t, v1.equals(v2))
}

func TestGetVersionFromReqLine(t *testing.T) {
	reqLines := "package1==3.2.1\npackage2==2.3.1"

	version, _ := getVersionFromReqLine("package1", reqLines)
	expectedVersion, _ := parseVersion("3.2.1")
	assert.Equal(t, expectedVersion, version)

	version, _ = getVersionFromReqLine("package2", reqLines)
	expectedVersion, _ = parseVersion("2.3.1")
	assert.Equal(t, expectedVersion, version)

	version, _ = getVersionFromReqLine("package3", reqLines)
	assert.Nil(t, version)

	// Add package2 a second time, should error out
	reqLines += "\npackage2==2.2.0"
	version, err := getVersionFromReqLine("package2", reqLines)
	assert.Nil(t, version)
	assert.NotNil(t, err)
}
