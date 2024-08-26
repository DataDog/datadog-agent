package npcollectorimpl

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name           string
		configYaml     string
		expectedConfig *collectorConfigs
	}{
		{
			name:       "default config",
			configYaml: "",
			expectedConfig: &collectorConfigs{
				connectionsMonitoringEnabled: false,
				workers:                      4,
				timeout:                      10 * time.Second,
				maxTTL:                       30,
				pathtestInputChanSize:        1000,
				pathtestProcessingChanSize:   1000,
				pathtestContextsLimit:        10000,
				pathtestTTL:                  15 * time.Minute,
				pathtestInterval:             5 * time.Minute,
				flushInterval:                10 * time.Second,
				networkDevicesNamespace:      "default",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentConfig.Datadog().SetConfigType("yaml")
			err := agentConfig.Datadog().ReadConfig(strings.NewReader(tt.configYaml))
			require.NoError(t, err)
			require.NoError(t, err)
			cfg := newConfig(agentConfig.Datadog())
			assert.Equal(t, tt.expectedConfig, cfg)
		})
	}
}

func TestNetworkPathCollectorEnabled(t *testing.T) {
	config := &collectorConfigs{
		connectionsMonitoringEnabled: true,
	}
	assert.True(t, config.networkPathCollectorEnabled())

	config.connectionsMonitoringEnabled = false
	assert.False(t, config.networkPathCollectorEnabled())
}
