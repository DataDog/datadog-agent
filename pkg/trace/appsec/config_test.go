// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/stretchr/testify/require"
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
		prepareArg     func() *config.AgentConfig
		expectedConfig *Config
		expectedError  bool
	}{
		{
			name: "default configuration",
			prepareArg: func() *config.AgentConfig {
				return config.New()
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "disabled",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.Enabled = false
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        false,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.MaxPayloadSize = -1
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.MaxPayloadSize = 0
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: defaultPayloadSize,
			},
		},
		{
			name: "max payload size",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.MaxPayloadSize = 1024
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 1024,
			},
		},
		{
			name: "api key",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.APIKey = "secret token"
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "secret token",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "intake url from site",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.Site = "my.site.com"
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      intakeURL("my.site.com"),
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "intake url from empty site",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.Site = ""
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      defaultIntakeURL,
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "config with bad site",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.Site = "not a site"
				return cfg
			},
			expectedConfig: nil,
			expectedError:  true,
		},
		{
			name: "intake url enforced by config",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.DDURL = "my.site/url"
				return cfg
			},
			expectedConfig: &Config{
				Enabled:        true,
				IntakeURL:      mustParseURL("my.site/url"),
				APIKey:         "",
				MaxPayloadSize: 5 * 1024 * 1024,
			},
		},
		{
			name: "bad intake url enforced by config",
			prepareArg: func() *config.AgentConfig {
				cfg := config.New()
				cfg.AppSec.DDURL = "http://my bad url"
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
