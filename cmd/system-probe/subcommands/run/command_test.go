// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"run"},
		run,
		func() {})
}

// envFunc builds a lookup function from a map, so tests never touch os env.
func envFunc(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestReadConfigstreamBootstrap(t *testing.T) {
	t.Run("defaults when env and yaml are empty", func(t *testing.T) {
		// Use an empty YAML at cliConfigPath so the function doesn't fall through
		// to a system-installed /etc or /opt datadog.yaml.
		dir := t.TempDir()
		path := filepath.Join(dir, "datadog.yaml")
		require.NoError(t, os.WriteFile(path, []byte(""), 0600))

		bs := readConfigstreamBootstrap(path, envFunc(nil))
		require.False(t, bs.Enabled)
		require.Equal(t, "localhost", bs.CmdHost)
		require.Equal(t, 5001, bs.CmdPort)
	})

	t.Run("yaml supplies custom cmd_host and cmd_port", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "datadog.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  configstream:
    consumer:
      enabled: true
`), 0600))

		bs := readConfigstreamBootstrap(path, envFunc(nil))
		require.True(t, bs.Enabled)
		require.Equal(t, "10.0.0.5", bs.CmdHost)
		require.Equal(t, 9000, bs.CmdPort)
	})

	t.Run("env vars override yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "datadog.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  configstream:
    consumer:
      enabled: false
`), 0600))

		env := envFunc(map[string]string{
			configstreamConsumerEnabledEnvVar: "true",
			"DD_CMD_HOST":                     "192.168.1.1",
			"DD_CMD_PORT":                     "7000",
		})

		bs := readConfigstreamBootstrap(path, env)
		require.True(t, bs.Enabled)
		require.Equal(t, "192.168.1.1", bs.CmdHost)
		require.Equal(t, 7000, bs.CmdPort)
	})
}
