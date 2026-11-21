// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestResolveProcmgrEnabled(t *testing.T) {
	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevel(&buf, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() {
		discard, discardErr := log.LoggerFromWriterWithMinLevel(io.Discard, log.WarnLvl)
		if discardErr == nil {
			log.SetupLogger(discard, "warn")
		}
	})

	tests := []struct {
		name       string
		process    string
		processSet bool
		install    string
		installSet bool
		yaml       *string
		missing    bool
		unreadable bool
		want       bool
		source     string
		wantWarn   bool
		wantErr    bool
	}{
		{name: "nothing set", want: true, source: procmgrSourceDefault},
		{name: "yaml flow false", yaml: strPtr("process_manager: {enabled: false}\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml block false", yaml: strPtr("process_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted false", yaml: strPtr("process_manager.enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted true", yaml: strPtr("api_key: abc\nprocess_manager.enabled: true\n"), want: true, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted case insensitive", yaml: strPtr("Process_Manager.Enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml nested case insensitive", yaml: strPtr("Process_Manager:\n  Enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml forms disagree nested true", yaml: strPtr("process_manager:\n  enabled: true\nprocess_manager.enabled: false\n"), want: false, source: procmgrSourceDatadogYAML, wantWarn: true},
		{name: "yaml forms disagree dotted true", yaml: strPtr("process_manager.enabled: true\nprocess_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML, wantWarn: true},
		{name: "yaml dotted null keeps nested false", yaml: strPtr("process_manager:\n  enabled: false\nprocess_manager.enabled:\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml nested null keeps dotted false", yaml: strPtr("process_manager.enabled: false\nprocess_manager:\n  enabled:\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted null", yaml: strPtr("process_manager.enabled:\n"), want: true, source: procmgrSourceDefault},
		{name: "yaml section without enabled keeps dotted false", yaml: strPtr("process_manager.enabled: false\nprocess_manager:\n  log_level: debug\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml duplicate enabled in one block", yaml: strPtr("process_manager:\n  enabled: true\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml scalar with dotted false is an error", yaml: strPtr("process_manager.enabled: false\nprocess_manager: true\n"), want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "yaml scalar with dotted true is an error", yaml: strPtr("process_manager.enabled: true\nprocess_manager: invalid\n"), want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "yaml top-level merge false", yaml: strPtr("defaults: &d\n  process_manager:\n    enabled: false\n<<: *d\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml nested merge false", yaml: strPtr("defaults: &d\n  enabled: false\nprocess_manager:\n  <<: *d\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted last key wins", yaml: strPtr("process_manager.enabled: true\nprocess_manager.enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted unquoted no", yaml: strPtr("process_manager.enabled: no\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted zero is off", yaml: strPtr("process_manager.enabled: 0\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml dotted quoted false", yaml: strPtr("process_manager.enabled: \"false\"\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml true", yaml: strPtr("process_manager:\n  enabled: true\n"), want: true, source: procmgrSourceDatadogYAML},
		{name: "yaml key commented out", yaml: strPtr("process_manager:\n  # enabled: false\n"), want: true, source: procmgrSourceDefault},
		{name: "yaml null", yaml: strPtr("process_manager:\n  enabled:\n"), want: true, source: procmgrSourceDefault},
		{name: "yaml unquoted no", yaml: strPtr("process_manager:\n  enabled: no\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml zero is off", yaml: strPtr("process_manager:\n  enabled: 0\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml quoted false", yaml: strPtr("process_manager:\n  enabled: \"false\"\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml quoted not a bool", yaml: strPtr("process_manager:\n  enabled: \"nope\"\n"), want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "yaml scalar", yaml: strPtr("process_manager: true\n"), want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "yaml last block wins", yaml: strPtr("process_manager:\n  enabled: true\nprocess_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml duplicate unrelated key", yaml: strPtr("api_key: a\napi_key: b\nprocess_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml unparsable", yaml: strPtr("process_manager: [\n"), want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "yaml unreadable", unreadable: true, want: false, source: procmgrSourceDatadogYAML, wantErr: true},
		{name: "missing file", missing: true, want: true, source: procmgrSourceDefault},
		{name: "install script beats unparsable yaml", yaml: strPtr("process_manager: [\n"), install: "true", installSet: true, want: true, source: procmgrSourceInstallScript},
		{name: "process env beats unparsable yaml", yaml: strPtr("process_manager: [\n"), process: "false", processSet: true, want: false, source: procmgrSourceEnvironment},
		{name: "install script beats yaml", yaml: strPtr("process_manager:\n  enabled: true\n"), install: "false", installSet: true, want: false, source: procmgrSourceInstallScript},
		{name: "install script true beats yaml false", yaml: strPtr("process_manager:\n  enabled: false\n"), install: "true", installSet: true, want: true, source: procmgrSourceInstallScript},
		{name: "process env beats install script and yaml", yaml: strPtr("process_manager:\n  enabled: true\n"), install: "true", installSet: true, process: "false", processSet: true, want: false, source: procmgrSourceEnvironment},
		{name: "process env false ignores case", process: "FALSE", processSet: true, want: false, source: procmgrSourceEnvironment},
		{name: "process env blank falls through", process: "  ", processSet: true, install: "false", installSet: true, want: false, source: procmgrSourceInstallScript},
		{name: "process env zero is on", process: "0", processSet: true, want: true, source: procmgrSourceEnvironment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			dir := t.TempDir()
			path := filepath.Join(dir, "datadog.yaml")
			switch {
			case tt.unreadable:
				require.NoError(t, os.Mkdir(path, 0o755))
			case tt.missing:
				path = filepath.Join(dir, "missing.yaml")
			case tt.yaml != nil:
				require.NoError(t, os.WriteFile(path, []byte(*tt.yaml), 0o644))
			default:
				path = filepath.Join(dir, "absent.yaml")
			}
			install := installEnv{}
			if tt.installSet {
				install[envProcessManagerEnabled] = tt.install
			}
			lookup := func(key string) (string, bool) {
				if key == envProcessManagerEnabled && tt.processSet {
					return tt.process, true
				}
				return "", false
			}

			enabled, source, resolveErr := resolveProcmgrEnabled(lookup, install, path)

			if tt.wantErr {
				require.Error(t, resolveErr)
			} else {
				require.NoError(t, resolveErr)
			}
			assert.Equal(t, tt.want, enabled)
			assert.Equal(t, tt.source, source)
			if tt.wantWarn {
				assert.NotEmpty(t, buf.String())
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func strPtr(s string) *string { return &s }
