// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates the systemd units for the installer.
package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
)

//go:embed gen
var genFS embed.FS

// TestGenerationIsUpToDate tests that the generated templates are up to date.
//
// You can update the templates by running `go generate` in the templates directory.
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}

	generated := filepath.Join(os.TempDir(), "gen")
	os.MkdirAll(generated, 0755)

	err := generate(generated)
	assert.NoError(t, err)
	newGeneratedFS := os.DirFS(generated)
	currentGeneratedFS, err := fs.Sub(genFS, "gen")
	assert.NoError(t, err)

	fixtures.AssertEqualFS(t, currentGeneratedFS, newGeneratedFS)
}

func TestYamlSingleQuote(t *testing.T) {
	assert.Equal(t, "'C:/Program Files/Datadog/Agent'", yamlSingleQuote("C:/Program Files/Datadog/Agent"))
	assert.Equal(t, "'C:/Program Files/Datadog/D''atadog Agent'", yamlSingleQuote("C:/Program Files/Datadog/D'atadog Agent"))
}

func TestWindowsProcessTemplateApostropheInInstallDir(t *testing.T) {
	out := mustRenderYAMLConfig("datadog-agent-process-windows.yaml", installerTemplateData{
		InstallDir: `C:/Program Files/D'atadog Agent`,
		EtcDir:     `C:/ProgramData/D'atadog`,
	})

	var doc struct {
		Command             string `yaml:"command"`
		ConditionPathExists string `yaml:"condition_path_exists"`
		ConditionConfigAny  []struct {
			Path string `yaml:"path"`
		} `yaml:"condition_config_any"`
	}
	require.NoError(t, yaml.Unmarshal(out, &doc))

	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/process-agent.exe`, doc.Command)
	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/process-agent.exe`, doc.ConditionPathExists)
	require.Len(t, doc.ConditionConfigAny, 2)
	assert.Equal(t, `C:/ProgramData/D'atadog/datadog.yaml`, doc.ConditionConfigAny[0].Path)
	assert.Equal(t, `C:/ProgramData/D'atadog/system-probe.yaml`, doc.ConditionConfigAny[1].Path)
}
