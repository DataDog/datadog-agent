package networkpath

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewCheckConfig(t *testing.T) {
	tests := []struct {
		name           string
		rawInstance    integration.Data
		initConfig     integration.Data
		expectedConfig CheckConfig
		expecteError   string
	}{
		{
			name:        "default config",
			rawInstance: []byte(`hostname: 1.2.3.4`),
			expectedConfig: CheckConfig{
				DestHostname: "1.2.3.4",
				DestPort:     0,
				MaxTTL:       30,
				TimeoutMs:    3000,
			},
		},
		{
			name: "custom config",
			rawInstance: []byte(`
hostname: 1.2.3.4
port: 42
max_ttl: 35
timeout: 1000
`),
			expectedConfig: CheckConfig{
				DestHostname: "1.2.3.4",
				DestPort:     42,
				MaxTTL:       35,
				TimeoutMs:    1000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewCheckConfig(tt.rawInstance, tt.initConfig)
			assert.Equal(t, &tt.expectedConfig, config)
			if tt.expecteError != "" {
				assert.EqualError(t, err, tt.expecteError)
			}
		})
	}
}
