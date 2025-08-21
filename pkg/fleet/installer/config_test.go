// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteConfigSymlinks(t *testing.T) {
	fleetDir := t.TempDir()
	userDir := t.TempDir()
	err := os.WriteFile(filepath.Join(userDir, "datadog.yaml"), []byte("user config"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(fleetDir, "datadog.yaml"), []byte("fleet config"), 0644)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Join(fleetDir, "conf.d"), 0755)
	assert.NoError(t, err)

	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.FileExists(t, filepath.Join(userDir, "conf.d.override"))
	configContent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml"))
	assert.NoError(t, err)
	overrideConfigConent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoError(t, err)
	assert.Equal(t, "user config", string(configContent))
	assert.Equal(t, "fleet config", string(overrideConfigConent))

	fleetDir = t.TempDir()
	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.NoFileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoFileExists(t, filepath.Join(userDir, "conf.d.override"))
}

