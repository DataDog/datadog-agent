// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package installinfo offers helpers to interact with the 'install_info' file.
//
// The install_info files is present next to the agent configuration and contains information about how the agent was//
// installed and its version history.  The file is automatically updated by installation tools (MSI installer, Chef,
// Ansible, DPKG, ...).
package installinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallSignature(t *testing.T) {
	installSigFile = filepath.Join(t.TempDir(), "install.json")
	testInstallType := "manual_update_via_apt"
	require.NoError(t, writeInstallSignature(testInstallType))

	content, err := os.ReadFile(installSigFile)
	if err != nil {
		require.NoError(t, err)
	}
	var installSignature map[string]string
	err = json.Unmarshal(content, &installSignature)
	if err != nil {
		require.NoError(t, err)
	}
	assert.Equal(t, 3, len(installSignature))
	installUUID := installSignature["install_id"]
	_, err = uuid.Parse(installUUID)
	assert.NoError(t, err)

	installType := installSignature["install_type"]
	assert.Equal(t, testInstallType, installType)

	installTime := installSignature["install_time"]
	unixInt, err := strconv.ParseInt(installTime, 10, 64)
	assert.NoError(t, err)
	diff := time.Now().Unix() - unixInt
	assert.True(t, diff*diff < 3600*3600)
}

func TestInstallMethod(t *testing.T) {
	installInfoFile = filepath.Join(t.TempDir(), "install_info")
	writeInstallInfo("dpkg", "1.2.3", "updater_package")
	resInstallInfo, err := getFromPath(installInfoFile)
	assert.NoError(t, err)

	assert.Equal(t, "updater_package", resInstallInfo.InstallerVersion)
	assert.Equal(t, "dpkg", resInstallInfo.Tool)
	assert.Equal(t, "1.2.3", resInstallInfo.ToolVersion)
	assert.NoError(t, err)
}

func TestDoubleWrite(t *testing.T) {
	tmpDir := t.TempDir()
	installInfoFile = filepath.Join(tmpDir, "install_info")
	installSigFile = filepath.Join(tmpDir, "install.json")

	s, _ := getFromPath(installInfoFile)
	assert.Nil(t, s)

	assert.NoError(t, WriteInstallInfo("v1", ""))
	v1, err := getFromPath(installInfoFile)
	assert.NoError(t, err)

	assert.NoError(t, WriteInstallInfo("v2", ""))
	v2, err := getFromPath(installInfoFile)
	assert.NoError(t, err)

	assert.Equal(t, v1, v2)
}

func TestRmInstallInfo(t *testing.T) {
	tmpDir := t.TempDir()
	installInfoFile = filepath.Join(tmpDir, "install_info")
	installSigFile = filepath.Join(tmpDir, "install.json")
	assert.NoError(t, WriteInstallInfo("v1", ""))

	assert.True(t, fileExists(installInfoFile))
	assert.True(t, fileExists(installSigFile))

	RmInstallInfo()
	assert.False(t, fileExists(installInfoFile))
	assert.False(t, fileExists(installSigFile))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
