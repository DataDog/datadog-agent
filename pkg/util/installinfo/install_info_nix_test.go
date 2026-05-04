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

	"go.yaml.in/yaml/v2"

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
	require.NoError(t, writeInstallInfo("dpkg", "1.2.3", "updater_package"))

	content, err := os.ReadFile(installInfoFile)
	require.NoError(t, err)

	var result struct {
		Method struct {
			Tool             string `yaml:"tool"`
			ToolVersion      string `yaml:"tool_version"`
			InstallerVersion string `yaml:"installer_version"`
		} `yaml:"install_method"`
	}
	require.NoError(t, yaml.UnmarshalStrict(content, &result))
	assert.Equal(t, "dpkg", result.Method.Tool)
	assert.Equal(t, "1.2.3", result.Method.ToolVersion)
	assert.Equal(t, "updater_package", result.Method.InstallerVersion)
}

func TestDoubleWrite(t *testing.T) {
	tmpDir := t.TempDir()
	installInfoFile = filepath.Join(tmpDir, "install_info")
	installSigFile = filepath.Join(tmpDir, "install.json")

	_, err := os.Stat(installInfoFile)
	assert.True(t, os.IsNotExist(err))

	assert.NoError(t, WriteInstallInfo("dpkg", "v1", ""))
	content1, err := os.ReadFile(installInfoFile)
	assert.NoError(t, err)

	// WriteInstallInfo is a no-op if the file already exists.
	assert.NoError(t, WriteInstallInfo("dpkg", "v2", ""))
	content2, err := os.ReadFile(installInfoFile)
	assert.NoError(t, err)

	assert.Equal(t, content1, content2)
}

func TestRmInstallInfo(t *testing.T) {
	tmpDir := t.TempDir()
	installInfoFile = filepath.Join(tmpDir, "install_info")
	installSigFile = filepath.Join(tmpDir, "install.json")
	assert.NoError(t, WriteInstallInfo("tool", "v1", ""))

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
