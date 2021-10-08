package appsec

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestConfig(t *testing.T) {
	intakeURL := func(site string) *url.URL {
		u, err := url.Parse(fmt.Sprintf(defaultIntakeURLTemplate, site))
		require.NoError(t, err)
		return u
	}
	defaultIntakeURL := intakeURL(defaultSite)
	mustParseURL := func(u string) *url.URL {
		url, err := url.Parse(u)
		require.NoError(t, err)
		return url
	}
	tests := []struct {
		name           string
		prepareArg     func() *config.MockConfig
		expectedConfig *Config
		expectedError  bool
	}{
		{
			name: "default configuration",
			prepareArg: func() *config.MockConfig {
				return config.Mock()
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "disabled",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.enabled", false)
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        false,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.max_payload_size", -1)
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: defaultPayloadSize,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.max_payload_size", 0)
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: defaultPayloadSize,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.max_payload_size", 1024)
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 1024,
			},
		},
		{
			name: "api key",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("api_key", "secret token")
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "secret token",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "intake url from site",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("site", "my.site.com")
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      intakeURL("my.site.com"),
				APIKey:         "",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "intake url from empty site",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("site", "")
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "config with bad site",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("site", "not a site")
				return cfg
			},
			expectedConfig: nil,
			expectedError:  true,
		},
		{
			name: "intake url enforced by config",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.appsec_dd_url", "my.site/url")
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        config.DefaultAppSecEnabled,
				IntakeURL:      mustParseURL("my.site/url"),
				APIKey:         "",
				MaxPayloadSize: config.DefaultAppSecMaxPayloadSize,
			},
		},
		{
			name: "bad intake url enforced by config",
			prepareArg: func() *config.MockConfig {
				cfg := config.Mock()
				cfg.Set("appsec_config.appsec_dd_url", "http://my bad url")
				return cfg
			},
			expectedConfig: nil,
			expectedError:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := newConfig(tt.prepareArg())
			require.Equal(t, tt.expectedConfig, cfg)
			if tt.expectedError {
				require.Error(t, err)
			}
		})
	}
}
