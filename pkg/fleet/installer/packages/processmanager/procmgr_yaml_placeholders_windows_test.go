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

// TestSubstituteProcmgrYAMLPlaceholdersApostropheInPaths asserts through yaml.Unmarshal
// rather than on the rendered string: an install root containing an apostrophe used to
// close the single-quoted scalar early, and the resulting catalog entry failed to parse
// with nothing naming the install path as the cause.
func TestSubstituteProcmgrYAMLPlaceholdersApostropheInPaths(t *testing.T) {
	out := substituteProcmgrYAMLPlaceholders(
		embedded.PARWindowsProcmgrConfig,
		"PAR",
		`C:\Program Files\D'atadog Agent`,
		`C:\ProgramData\D'atadog`,
	)

	var doc struct {
		Command             string   `yaml:"command"`
		Args                []string `yaml:"args"`
		ConditionPathExists string   `yaml:"condition_path_exists"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))

	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/privateactionrunner.exe`, doc.Command)
	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/privateactionrunner.exe`, doc.ConditionPathExists)
	assert.Contains(t, doc.Args, `C:/ProgramData/D'atadog/datadog.yaml`)
}
