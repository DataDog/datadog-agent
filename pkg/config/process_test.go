package config

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// allProcessSettings is a slice that contains details for testing regarding process agent config settings
// When adding to this list please try to conform to the same ordering that is in `process.go`

var allProcessSettings = []struct {
	key          string
	defaultValue interface{}
}{
	{
		key:          "process_config.dd_agent_bin",
		defaultValue: defaultDDAgentBin,
	},
	{
		key:          "process_config.log_file",
		defaultValue: defaultProcessAgentLogFile,
	},
	{
		key:          "process_config.grpc_connection_timeout_secs",
		defaultValue: DefaultGRPCConnectionTimeoutSecs,
	},
	{
		key:          "process_config.remote_tagger",
		defaultValue: true,
	},
	{
		key:          "process_config.process_discovery.enabled",
		defaultValue: false,
	},
	{
		key:          "process_config.process_discovery.interval",
		defaultValue: 4 * time.Hour,
	},
}

// TestProcessDefaults tests to ensure that the config has set process settings correctly
func TestProcessConfig(t *testing.T) {
	cfg := setupConf()

	for _, tc := range allProcessSettings {
		t.Run(tc.key+" default", func(t *testing.T) {
			assert.Equal(t, tc.defaultValue, cfg.Get(tc.key))
		})
	}
}

// TestPrefixes tests that for every corresponding `DD_PROCESS_CONFIG` prefix, there is a `DD_PROCESS_AGENT` prefix as well.
func TestPrefixes(t *testing.T) {
	envVarSlice := setupConf().GetEnvVars()
	envVars := make(map[string]struct{}, len(envVarSlice))
	for _, envVar := range envVarSlice {
		envVars[envVar] = struct{}{}
	}

	for envVar := range envVars {
		if !strings.HasPrefix(envVar, "DD_PROCESS_CONFIG") {
			continue
		}

		processAgentEnvVar := strings.Replace(envVar, "PROCESS_CONFIG", "PROCESS_AGENT", 1)
		t.Run(fmt.Sprintf("%s and %s", envVar, processAgentEnvVar), func(t *testing.T) {
			// Check to see if envVars contains processAgentEnvVar. We can't use assert.Contains,
			// because when it fails the library prints all of envVars which is too noisy
			_, ok := envVars[processAgentEnvVar]
			assert.Truef(t, ok, "%s is defined but not %s", envVar, processAgentEnvVar)
		})
	}
}
