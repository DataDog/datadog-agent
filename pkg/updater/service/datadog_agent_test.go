// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

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
	"gopkg.in/yaml.v2"
)

func TestInstallSignature(t *testing.T) {
	installSigFile = filepath.Join(t.TempDir(), "/install.json")
	require.Nil(t, writeInstallSignature())

	content, err := os.ReadFile(installSigFile)
	if err != nil {
		require.Nil(t, err)
	}
	var installSignature map[string]string
	err = json.Unmarshal(content, &installSignature)
	if err != nil {
		require.Nil(t, err)
	}
	assert.Equal(t, 3, len(installSignature))
	installUUID := installSignature["install_id"]
	_, err = uuid.Parse(installUUID)
	assert.Nil(t, err)

	installType := installSignature["install_type"]
	assert.Equal(t, "manual_update_via_apt", installType)

	installTime := installSignature["install_time"]
	unixInt, err := strconv.ParseInt(installTime, 10, 64)
	assert.Nil(t, err)
	diff := time.Now().Unix() - unixInt
	assert.True(t, diff*diff < 3600*3600)
}

func TestInstallMethod(t *testing.T) {
	installInfoFile = filepath.Join(t.TempDir(), "install_info")
	writeInstallInfo("dpkg", "1.2.3")
	rawYaml, err := os.ReadFile(installInfoFile)
	assert.Nil(t, err)
	var config Config
	assert.Nil(t, yaml.Unmarshal(rawYaml, &config))

	assert.Equal(t, "updater", config.InstallMethod["installer"])
	assert.Equal(t, "updater_package", config.InstallMethod["installer_version"])
	assert.Equal(t, "dpkg", config.InstallMethod["tool"])
	assert.Equal(t, "1.2.3", config.InstallMethod["tool_version"])
	assert.Nil(t, err)
}

// Config yaml struct
type Config struct {
	InstallMethod map[string]string `yaml:"install_method"`
}
