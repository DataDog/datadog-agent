// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

func TestYamlSingleQuoteContent(t *testing.T) {
	assert.Equal(t, `C:/Program Files/Datadog/Agent`, yamlSingleQuoteContent(`C:/Program Files/Datadog/Agent`))
	assert.Equal(t, `C:/Program Files/D''atadog Agent`, yamlSingleQuoteContent(`C:/Program Files/D'atadog Agent`))
}

func TestSubstituteProcmgrYAMLPlaceholdersApostropheInPaths(t *testing.T) {
	out := substituteProcmgrYAMLPlaceholders(
		embedded.ProcessWindowsProcmgrConfig,
		"PROCESS",
		`C:\Program Files\D'atadog Agent`,
		`C:\ProgramData\D'atadog`,
	)

	var doc struct {
		Command             string `yaml:"command"`
		ConditionPathExists string `yaml:"condition_path_exists"`
		ConditionConfigAny  []struct {
			Path string `yaml:"path"`
		} `yaml:"condition_config_any"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))

	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/process-agent.exe`, doc.Command)
	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/process-agent.exe`, doc.ConditionPathExists)
	require.Len(t, doc.ConditionConfigAny, 2)
	assert.Equal(t, `C:/ProgramData/D'atadog/datadog.yaml`, doc.ConditionConfigAny[0].Path)
	assert.Equal(t, `C:/ProgramData/D'atadog/system-probe.yaml`, doc.ConditionConfigAny[1].Path)
}
