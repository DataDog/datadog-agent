// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// yamlSeedCases are the datadog.yaml cases of PR 60845 (datadog_agent_darwin_procmgr_test.go, commit a1419a0)
// that set only the file, mapped to SeedFromDatadogYAML: its "default" source is Found=false. Its
// environment and install-script cases are resolver tests for a later phase and are not ported.
// The extra cases come from section 7 of the Phase 1 spec.
var yamlSeedCases = []struct {
	name        string
	yaml        string
	missing     bool // the file does not exist
	unreadable  bool // the path is a directory
	wantFound   bool
	wantEnabled bool
	wantWarning bool
	wantErr     bool
}{
	// Ported from PR 60845.
	{name: "nothing set", yaml: "", wantFound: false},
	{name: "yaml flow false", yaml: "process_manager: {enabled: false}\n", wantFound: true, wantEnabled: false},
	{name: "yaml block false", yaml: "process_manager:\n  enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted false", yaml: "process_manager.enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted true", yaml: "api_key: abc\nprocess_manager.enabled: true\n", wantFound: true, wantEnabled: true},
	{name: "yaml dotted case insensitive", yaml: "Process_Manager.Enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml nested case insensitive", yaml: "Process_Manager:\n  Enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml forms disagree nested true", yaml: "process_manager:\n  enabled: true\nprocess_manager.enabled: false\n", wantFound: true, wantEnabled: false, wantWarning: true},
	{name: "yaml forms disagree dotted true", yaml: "process_manager.enabled: true\nprocess_manager:\n  enabled: false\n", wantFound: true, wantEnabled: false, wantWarning: true},
	{name: "yaml dotted null keeps nested false", yaml: "process_manager:\n  enabled: false\nprocess_manager.enabled:\n", wantFound: true, wantEnabled: false},
	{name: "yaml nested null keeps dotted false", yaml: "process_manager.enabled: false\nprocess_manager:\n  enabled:\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted null", yaml: "process_manager.enabled:\n", wantFound: false},
	{name: "yaml section without enabled keeps dotted false", yaml: "process_manager.enabled: false\nprocess_manager:\n  log_level: debug\n", wantFound: true, wantEnabled: false},
	{name: "yaml duplicate enabled in one block", yaml: "process_manager:\n  enabled: true\n  enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml scalar with dotted false is an error", yaml: "process_manager.enabled: false\nprocess_manager: true\n", wantErr: true},
	{name: "yaml scalar with dotted true is an error", yaml: "process_manager.enabled: true\nprocess_manager: invalid\n", wantErr: true},
	{name: "yaml top-level merge false", yaml: "defaults: &d\n  process_manager:\n    enabled: false\n<<: *d\n", wantFound: true, wantEnabled: false},
	{name: "yaml nested merge false", yaml: "defaults: &d\n  enabled: false\nprocess_manager:\n  <<: *d\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted last key wins", yaml: "process_manager.enabled: true\nprocess_manager.enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted unquoted no", yaml: "process_manager.enabled: no\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted zero is off", yaml: "process_manager.enabled: 0\n", wantFound: true, wantEnabled: false},
	{name: "yaml dotted quoted false", yaml: "process_manager.enabled: \"false\"\n", wantFound: true, wantEnabled: false},
	{name: "yaml true", yaml: "process_manager:\n  enabled: true\n", wantFound: true, wantEnabled: true},
	{name: "yaml key commented out", yaml: "process_manager:\n  # enabled: false\n", wantFound: false},
	{name: "yaml null", yaml: "process_manager:\n  enabled:\n", wantFound: false},
	{name: "yaml unquoted no", yaml: "process_manager:\n  enabled: no\n", wantFound: true, wantEnabled: false},
	{name: "yaml zero is off", yaml: "process_manager:\n  enabled: 0\n", wantFound: true, wantEnabled: false},
	{name: "yaml quoted false", yaml: "process_manager:\n  enabled: \"false\"\n", wantFound: true, wantEnabled: false},
	{name: "yaml quoted not a bool", yaml: "process_manager:\n  enabled: \"nope\"\n", wantErr: true},
	{name: "yaml scalar", yaml: "process_manager: true\n", wantErr: true},
	{name: "yaml last block wins", yaml: "process_manager:\n  enabled: true\nprocess_manager:\n  enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml duplicate unrelated key", yaml: "api_key: a\napi_key: b\nprocess_manager:\n  enabled: false\n", wantFound: true, wantEnabled: false},
	{name: "yaml unparsable", yaml: "process_manager: [\n", wantErr: true},
	{name: "yaml unreadable", unreadable: true, wantErr: true},
	{name: "missing file", missing: true, wantFound: false},

	// Extra cases from the spec.
	{name: "empty file", yaml: "", wantFound: false},
	{name: "unrelated keys only", yaml: "api_key: abc\nsite: datadoghq.eu\n", wantFound: false},
	{name: "mixed-case dotted and nested", yaml: "PROCESS_MANAGER.ENABLED: true\nprocess_manager:\n  ENABLED: false\n", wantFound: true, wantEnabled: false, wantWarning: true},
	{name: "merge key carries the opt-out", yaml: "base: &base\n  enabled: false\nprocess_manager:\n  <<: *base\n", wantFound: true, wantEnabled: false},
	{name: "null in one form and bool in the other", yaml: "process_manager:\n  enabled:\nprocess_manager.enabled: true\n", wantFound: true, wantEnabled: true},
	{name: "quoted false", yaml: "process_manager.enabled: \"false\"\n", wantFound: true, wantEnabled: false},
	{name: "non-bool string", yaml: "process_manager.enabled: maybe\n", wantErr: true},
	{name: "scalar process_manager foo", yaml: "process_manager: foo\n", wantErr: true},
	{name: "conflicting forms", yaml: "process_manager.enabled: true\nprocess_manager:\n  enabled: false\n", wantFound: true, wantEnabled: false, wantWarning: true},
	{name: "agreeing forms have no warning", yaml: "process_manager.enabled: true\nprocess_manager:\n  enabled: true\n", wantFound: true, wantEnabled: true},
	{name: "syntax error", yaml: "process_manager: [\n", wantErr: true},
	{name: "path is a directory", unreadable: true, wantErr: true},
}

func TestSeedFromDatadogYAML(t *testing.T) {
	for _, tt := range yamlSeedCases {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "datadog.yaml")
			switch {
			case tt.unreadable:
				require.NoError(t, os.Mkdir(path, 0o755))
			case tt.missing:
				path = filepath.Join(dir, "missing.yaml")
			default:
				require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o644))
			}

			res, err := SeedFromDatadogYAML(path)

			assert.Equal(t, path, res.Detail, "Detail is the path even when the knob is absent or broken")
			if tt.wantErr {
				var seedErr *SeedError
				require.ErrorAs(t, err, &seedErr)
				assert.Equal(t, path, seedErr.Detail)
				assert.False(t, res.Found)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, res.Found)
			if tt.wantFound {
				assert.Equal(t, tt.wantEnabled, res.Enabled)
			}
			if tt.wantWarning {
				assert.NotEmpty(t, res.Warning)
			} else {
				assert.Empty(t, res.Warning)
			}
		})
	}
}

func TestSeedFromDatadogYAMLConflictWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte("process_manager.enabled: true\nprocess_manager:\n  enabled: false\n"), 0o644))

	res, err := SeedFromDatadogYAML(path)

	require.NoError(t, err)
	assert.True(t, res.Found)
	assert.False(t, res.Enabled)
	assert.Equal(t, "process_manager.enabled is both true and false", res.Warning)
}

func TestSeedFromDatadogYAMLFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	require.NoError(t, os.WriteFile(target, []byte("process_manager.enabled: false\n"), 0o644))
	link := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.Symlink(target, link))

	res, err := SeedFromDatadogYAML(link)

	require.NoError(t, err)
	assert.True(t, res.Found)
	assert.False(t, res.Enabled)
}
