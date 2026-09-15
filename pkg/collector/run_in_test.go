// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"
)

// runInHolder mirrors how commonInitConfig and commonInstanceConfig read the
// option: one field among others, unmarshalled from a whole YAML document.
type runInHolder struct {
	RunIn RunIn  `yaml:"run_in"`
	Name  string `yaml:"name"`
}

func unmarshalRunIn(t *testing.T, doc string) runInHolder {
	t.Helper()
	var holder runInHolder
	require.NoError(t, yaml.Unmarshal([]byte(doc), &holder))
	return holder
}

func TestRunInUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		doc         string
		wantSet     bool
		wantRunners []string
		wantParseer bool
	}{
		{
			name:    "absent",
			doc:     "name: web\n",
			wantSet: false,
		},
		{
			name:        "scalar",
			doc:         "run_in: check_runner\n",
			wantSet:     true,
			wantRunners: []string{"check_runner"},
		},
		{
			name:        "inline list",
			doc:         "run_in: [core_agent, check_runner]\n",
			wantSet:     true,
			wantRunners: []string{"core_agent", "check_runner"},
		},
		{
			name:        "block list",
			doc:         "run_in:\n  - core_agent\n",
			wantSet:     true,
			wantRunners: []string{"core_agent"},
		},
		{
			name:        "normalizes case and whitespace",
			doc:         "run_in: '  Core_Agent  '\n",
			wantSet:     true,
			wantRunners: []string{"core_agent"},
		},
		{
			name:        "drops empty entries",
			doc:         "run_in: ['', core_agent, '  ']\n",
			wantSet:     true,
			wantRunners: []string{"core_agent"},
		},
		{
			name:        "empty list",
			doc:         "run_in: []\n",
			wantSet:     true,
			wantRunners: []string{},
		},
		{
			name:        "blank scalar names no runner",
			doc:         "run_in: ''\n",
			wantSet:     true,
			wantRunners: []string{},
		},
		{
			name:        "mapping is recorded as malformed, not an error",
			doc:         "run_in:\n  runner: check_runner\n",
			wantSet:     true,
			wantParseer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holder := unmarshalRunIn(t, tt.doc)
			assert.Equal(t, tt.wantSet, holder.RunIn.set)
			assert.Equal(t, tt.wantParseer, holder.RunIn.parseErr != "")
			if !tt.wantParseer && tt.wantSet {
				assert.Equal(t, tt.wantRunners, holder.RunIn.Runners())
			}
		})
	}
}

// A malformed run_in must never make the surrounding instance unparseable:
// CheckScheduler.getChecks drops the whole instance when yaml.Unmarshal fails.
func TestRunInNeverFailsTheEnclosingUnmarshal(t *testing.T) {
	for _, doc := range []string{
		"name: web\nrun_in:\n  runner: check_runner\n",
		"name: web\nrun_in: 42\n",
		"name: web\nrun_in: [1, 2]\n",
		"name: web\nrun_in: true\n",
	} {
		var holder runInHolder
		require.NoError(t, yaml.Unmarshal([]byte(doc), &holder), "doc: %s", doc)
		assert.Equal(t, "web", holder.Name, "sibling fields must survive; doc: %s", doc)
	}
}

func TestEvaluateRunIn(t *testing.T) {
	tests := []struct {
		name        string
		initConfig  string
		instance    string
		wantRun     bool
		wantWarning bool
	}{
		{
			name:       "absent everywhere runs on the core agent",
			initConfig: "{}",
			instance:   "name: web\n",
			wantRun:    true,
		},
		{
			name:       "init_config delegates",
			initConfig: "run_in: check_runner\n",
			instance:   "name: web\n",
			wantRun:    false,
		},
		{
			name:       "init_config keeps it here",
			initConfig: "run_in: core_agent\n",
			instance:   "name: web\n",
			wantRun:    true,
		},
		{
			name:       "instance overrides init_config to delegate",
			initConfig: "run_in: core_agent\n",
			instance:   "run_in: check_runner\n",
			wantRun:    false,
		},
		{
			name:       "instance overrides init_config to keep it here",
			initConfig: "run_in: check_runner\n",
			instance:   "run_in: core_agent\n",
			wantRun:    true,
		},
		{
			name:       "instance without the option inherits init_config",
			initConfig: "run_in: check_runner\n",
			instance:   "name: web\nmin_collection_interval: 30\n",
			wantRun:    false,
		},
		{
			name:        "unknown runner skips here and warns",
			initConfig:  "{}",
			instance:    "run_in: check_runnr\n",
			wantRun:     false,
			wantWarning: true,
		},
		{
			name:       "several runners including core_agent run here",
			initConfig: "{}",
			instance:   "run_in: [core_agent, check_runner]\n",
			wantRun:    true,
		},
		{
			name:       "several runners without core_agent skip here",
			initConfig: "{}",
			instance:   "run_in: [check_runner, future_runner]\n",
			wantRun:    false,
		},
		{
			name:        "several unknown runners skip here and warn",
			initConfig:  "{}",
			instance:    "run_in: [foo, bar]\n",
			wantRun:     false,
			wantWarning: true,
		},
		{
			name:        "malformed falls back to the core agent",
			initConfig:  "{}",
			instance:    "run_in:\n  runner: check_runner\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			name:        "empty list is an error, not an instruction",
			initConfig:  "{}",
			instance:    "run_in: []\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			name:        "blank name falls back to the core agent",
			initConfig:  "{}",
			instance:    "run_in: ''\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			name:        "list of blank names falls back to the core agent",
			initConfig:  "{}",
			instance:    "run_in: ['', '  ']\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			// yaml.v2 does not call a custom unmarshaller for an explicit null,
			// so `run_in:` with no value is indistinguishable from an absent key.
			name:       "null value is treated as absent",
			initConfig: "{}",
			instance:   "run_in:\n",
			wantRun:    true,
		},
		{
			name:        "instance empty list cancels an init_config runner",
			initConfig:  "run_in: check_runner\n",
			instance:    "run_in: []\n",
			wantRun:     false,
			wantWarning: true,
		},
		{
			name:        "instance empty list cancels a core_agent init_config",
			initConfig:  "run_in: core_agent\n",
			instance:    "run_in: ''\n",
			wantRun:     false,
			wantWarning: true,
		},
		{
			name:        "malformed instance cancels an init_config runner too",
			initConfig:  "run_in: check_runner\n",
			instance:    "run_in:\n  runner: check_runner\n",
			wantRun:     false,
			wantWarning: true,
		},
		{
			name:        "unusable instance without an init_config directive runs here",
			initConfig:  "{}",
			instance:    "run_in: []\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			name:        "unusable instance over an unusable init_config runs here",
			initConfig:  "run_in: []\n",
			instance:    "run_in: ''\n",
			wantRun:     true,
			wantWarning: true,
		},
		{
			name:        "unusable init_config falls through to the core agent",
			initConfig:  "run_in: []\n",
			instance:    "name: web\n",
			wantRun:     true,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initConfig := unmarshalRunIn(t, tt.initConfig)
			instance := unmarshalRunIn(t, tt.instance)

			decision := evaluateRunIn(initConfig.RunIn, instance.RunIn)

			assert.Equal(t, tt.wantRun, decision.run)
			assert.Equal(t, tt.wantWarning, decision.warning != "", "warning was %q", decision.warning)
		})
	}
}
