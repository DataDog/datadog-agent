// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsunits

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestRenderReplacesEveryPlaceholder(t *testing.T) {
	for _, u := range All() {
		if u.Template == "" {
			continue
		}
		t.Run(u.Name, func(t *testing.T) {
			content, err := Render(u, "C:/Program Files/Datadog/Datadog Agent", "C:/ProgramData/Datadog")
			require.NoError(t, err)
			assert.NotContains(t, string(content), "__")
			assert.NotContains(t, string(content), "{{")
		})
	}
}

// TestRenderEscapesApostrophesInRoots asserts through yaml.Unmarshal rather than on the rendered
// string: an install root containing an apostrophe used to close the single-quoted scalar early,
// and the resulting catalog entry failed to parse with nothing naming the install path as the cause.
func TestRenderEscapesApostrophesInRoots(t *testing.T) {
	out, err := Render(Procmgr.Action, `C:/Program Files/D'atadog Agent`, `C:/ProgramData/D'atadog`)
	require.NoError(t, err)

	var doc struct {
		Command             string   `yaml:"command"`
		Args                []string `yaml:"args"`
		ConditionPathExists string   `yaml:"condition_path_exists"`
	}
	require.NoError(t, yaml.Unmarshal(out, &doc))

	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/privateactionrunner.exe`, doc.Command)
	assert.Equal(t, `C:/Program Files/D'atadog Agent/bin/agent/privateactionrunner.exe`, doc.ConditionPathExists)
	assert.Contains(t, doc.Args, `C:/ProgramData/D'atadog/datadog.yaml`)
}
