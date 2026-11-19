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
		want       bool
		source     string
		wantWarn   bool
	}{
		{name: "nothing set", want: true, source: procmgrSourceDefault},
		{name: "yaml flow false", yaml: strPtr("process_manager: {enabled: false}\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml block false", yaml: strPtr("process_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml true", yaml: strPtr("process_manager:\n  enabled: true\n"), want: true, source: procmgrSourceDatadogYAML},
		{name: "yaml key commented out", yaml: strPtr("process_manager:\n  # enabled: false\n"), want: true, source: procmgrSourceDefault},
		{name: "yaml null", yaml: strPtr("process_manager:\n  enabled:\n"), want: true, source: procmgrSourceDefault},
		{name: "yaml unquoted no", yaml: strPtr("process_manager:\n  enabled: no\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml not a bool", yaml: strPtr("process_manager:\n  enabled: 0\n"), want: true, source: procmgrSourceDefault, wantWarn: true},
		{name: "yaml scalar", yaml: strPtr("process_manager: true\n"), want: true, source: procmgrSourceDefault, wantWarn: true},
		{name: "yaml last block wins", yaml: strPtr("process_manager:\n  enabled: true\nprocess_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml duplicate unrelated key", yaml: strPtr("api_key: a\napi_key: b\nprocess_manager:\n  enabled: false\n"), want: false, source: procmgrSourceDatadogYAML},
		{name: "yaml unparsable", yaml: strPtr("process_manager: [\n"), want: true, source: procmgrSourceDefault, wantWarn: true},
		{name: "missing file", missing: true, want: true, source: procmgrSourceDefault},
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
			if tt.missing {
				path = filepath.Join(dir, "missing.yaml")
			} else if tt.yaml != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.yaml), 0o644))
			} else {
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

			enabled, source := resolveProcmgrEnabled(lookup, install, path)

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
