// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installinfo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestWriteInstallInfo(t *testing.T) {
	// To avoid flakiness, remove dpkg & rpm from path, if any
	oldPath := os.Getenv("PATH")
	defer func() { os.Setenv("PATH", oldPath) }()
	os.Setenv("PATH", "")

	tmpDir := t.TempDir()
	infoPath := filepath.Join(tmpDir, "install_info")
	sigPath := filepath.Join(tmpDir, "install.json")
	testInstallType := "deb"

	// Call the internal writeInstallInfo function.
	time := time.Now()
	uuid := uuid.New().String()
	err := writeInstallInfo(context.TODO(), infoPath, sigPath, testInstallType, time, uuid)
	require.NoError(t, err)

	yamlData, err := os.ReadFile(infoPath)
	require.NoError(t, err)

	var info map[string]map[string]string
	err = yaml.Unmarshal(yamlData, &info)
	require.NoError(t, err)

	expectedYAML := map[string]map[string]string{
		"install_method": {
			"tool":              "installer",
			"tool_version":      version.AgentVersion,
			"installer_version": testInstallType + "_package",
		},
	}
	assert.Equal(t, expectedYAML, info)

	jsonData, err := os.ReadFile(sigPath)
	require.NoError(t, err)

	var sig map[string]string
	err = json.Unmarshal(jsonData, &sig)
	require.NoError(t, err)

	expectedSig := map[string]string{
		"install_id":   uuid,
		"install_type": testInstallType + "_package",
		"install_time": strconv.FormatInt(time.Unix(), 10),
	}
	assert.Equal(t, expectedSig, sig)
}
