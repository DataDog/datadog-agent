// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		path   string
		want   string
	}{
		{
			name:   "leading slash single segment",
			prefix: "/opt/datadog-agent",
			path:   "/run",
			want:   filepath.Join("/opt/datadog-agent", "run"),
		},
		{
			name:   "no leading slash single segment",
			prefix: "/opt/datadog-agent",
			path:   "run",
			want:   filepath.Join("/opt/datadog-agent", "run"),
		},
		{
			name:   "leading slash nested segments",
			prefix: "/etc/datadog-agent",
			path:   "/conf.d/foo",
			want:   filepath.Join("/etc/datadog-agent", "conf.d", "foo"),
		},
		{
			name:   "no leading slash nested segments",
			prefix: "/etc/datadog-agent",
			path:   "conf.d/foo",
			want:   filepath.Join("/etc/datadog-agent", "conf.d", "foo"),
		},
		{
			name:   "file name",
			prefix: "/var/log/datadog",
			path:   "/agent.log",
			want:   filepath.Join("/var/log/datadog", "agent.log"),
		},
		{
			name:   "trailing slash is cleaned",
			prefix: "/opt/datadog-agent",
			path:   "/run/",
			want:   filepath.Join("/opt/datadog-agent", "run"),
		},
		{
			name:   "empty path returns prefix",
			prefix: "/opt/datadog-agent",
			path:   "",
			want:   filepath.Join("/opt/datadog-agent"),
		},
		{
			name:   "relative prefix is preserved",
			prefix: "opt/datadog-agent",
			path:   "/run",
			want:   filepath.Join("opt/datadog-agent", "run"),
		},
		{
			name:   "Windows Path",
			prefix: "C:\\programdata\\Datadog\\Agent",
			path:   "/logs/agent.log",
			want:   filepath.Join("C:\\programdata\\Datadog\\Agent", "logs", "agent.log"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolvePath(tc.prefix, tc.path))
		})
	}
}

// newDefaultsConfig returns a config with the given default settings. The schema
// is not built yet, so the caller decides when to call BuildSchema (which is what
// resolves the relative paths).
func newDefaultsConfig(t *testing.T, defaults map[string]interface{}) *ntmConfig {
	t.Helper()
	cfg := NewNodeTreeConfig("test", "TEST", nil).(*ntmConfig)
	for key, val := range defaults {
		cfg.SetDefault(key, val)
	}
	return cfg
}

func TestResolveRelativePath(t *testing.T) {
	c := newDefaultsConfig(t, map[string]interface{}{
		"conf_file":      "${conf_path}/datadog.yaml",
		"install_socket": "${install_path}/run/runtime-security.sock",
		"run_socket":     "${run_path}/agent_ipc.socket",
		"log_file":       "${log_path}/agent.log",
		// bare token with no trailing path resolves to the prefix itself
		"run_dir": "${run_path}",
		// nested setting using the relative notation
		"foo.bar": "${run_path}/baz",
		// plain strings and non-string values must be left untouched
		"host": "localhost",
		"port": 8080,
	})

	// BuildSchema resolves the relative paths using the runtime default paths.
	c.BuildSchema()

	assert.Equal(t, filepath.Join(defaultpaths.GetDefaultConfPath(), "datadog.yaml"), c.Get("conf_file"))
	assert.Equal(t, filepath.Join(defaultpaths.GetInstallPath(), "run", "runtime-security.sock"), c.Get("install_socket"))
	assert.Equal(t, filepath.Join(defaultpaths.GetDefaultRunPath(), "agent_ipc.socket"), c.Get("run_socket"))
	assert.Equal(t, filepath.Join(defaultpaths.GetDefaultLogPath(), "agent.log"), c.Get("log_file"))
	assert.Equal(t, defaultpaths.GetDefaultRunPath(), c.Get("run_dir"))
	assert.Equal(t, filepath.Join(defaultpaths.GetDefaultRunPath(), "baz"), c.Get("foo.bar"))

	// untouched values
	assert.Equal(t, "localhost", c.Get("host"))
	assert.Equal(t, 8080, c.Get("port"))
}

func TestResolveRelativePathInvalidPrefix(t *testing.T) {
	c := newDefaultsConfig(t, map[string]interface{}{
		"bad_setting": "${unknown_path}/foo",
	})

	// An unknown token is reported as an error and the value is left untouched.
	err := c.resolveRelativePath("/conf", "/install", "/run", "/log")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad_setting")
	assert.Equal(t, "${unknown_path}/foo", c.leafAtPathFromNode("bad_setting", c.defaults).Get())
}

func TestResolveRelativePathNoMatch(t *testing.T) {
	// Values that don't start with the ${...} notation must be left as-is.
	c := newDefaultsConfig(t, map[string]interface{}{
		"absolute": "/var/log/datadog/agent.log",
		"empty":    "",
		// a ${...} token that isn't at the start of the value is not a relative path
		"inline": "prefix-${run_path}",
	})

	c.BuildSchema()

	assert.Equal(t, "/var/log/datadog/agent.log", c.Get("absolute"))
	assert.Equal(t, "", c.Get("empty"))
	assert.Equal(t, "prefix-${run_path}", c.Get("inline"))
}
