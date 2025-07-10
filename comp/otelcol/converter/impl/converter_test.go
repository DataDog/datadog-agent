// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package converterimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.uber.org/zap"
)

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func newResolver(uris []string) (*confmap.Resolver, error) {
	return confmap.NewResolver(confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		ConverterFactories: []confmap.ConverterFactory{},
		DefaultScheme:      "env",
	})
}

func TestNewConverterForAgent(t *testing.T) {
	_, err := NewConverterForAgent(Requires{})
	assert.NoError(t, err)
}

func TestConvert(t *testing.T) {
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_SITE", "")
	tests := []struct {
		name           string
		provided       string
		expectedResult string
		agentConfig    string
	}{
		{
			name:           "extensions/empty-extensions",
			provided:       "extensions/empty-extensions/config.yaml",
			expectedResult: "extensions/empty-extensions/config-result.yaml",
		},
		{
			name:           "extensions/no-extensions",
			provided:       "extensions/no-extensions/config.yaml",
			expectedResult: "extensions/no-extensions/config-result.yaml",
		},
		{
			name:           "extensions/other-extensions",
			provided:       "extensions/other-extensions/config.yaml",
			expectedResult: "extensions/other-extensions/config-result.yaml",
		},
		{
			name:           "extensions/no-changes",
			provided:       "extensions/no-changes/config.yaml",
			expectedResult: "extensions/no-changes/config.yaml",
		},
		{
			name:           "processors/empty-processors",
			provided:       "processors/empty-processors/config.yaml",
			expectedResult: "processors/empty-processors/config-result.yaml",
		},
		{
			name:           "processors/no-processors",
			provided:       "processors/no-processors/config.yaml",
			expectedResult: "processors/no-processors/config-result.yaml",
		},
		{
			name:           "processors/other-processors",
			provided:       "processors/other-processors/config.yaml",
			expectedResult: "processors/other-processors/config-result.yaml",
		},
		{
			name:           "processors/no-processor-partial",
			provided:       "processors/no-processor-partial/config.yaml",
			expectedResult: "processors/no-processor-partial/config-result.yaml",
		},
		{
			name:           "processors/no-changes",
			provided:       "processors/no-changes/config.yaml",
			expectedResult: "processors/no-changes/config.yaml",
		},
		{
			name:           "receivers/empty-receivers",
			provided:       "receivers/empty-receivers/config.yaml",
			expectedResult: "receivers/empty-receivers/config-result.yaml",
		},
		{
			name:           "receivers/job-name-change",
			provided:       "receivers/job-name-change/config.yaml",
			expectedResult: "receivers/job-name-change/config-result.yaml",
		},
		{
			name:           "receivers/no-changes",
			provided:       "receivers/no-changes/config.yaml",
			expectedResult: "receivers/no-changes/config.yaml",
		},
		{
			name:           "receivers/no-changes-multiple-dd",
			provided:       "receivers/no-changes-multiple-dd/config.yaml",
			expectedResult: "receivers/no-changes-multiple-dd/config.yaml",
		},
		{
			name:           "receivers/no-changes-multiple-dd-same-pipeline",
			provided:       "receivers/no-changes-multiple-dd-same-pipeline/config.yaml",
			expectedResult: "receivers/no-changes-multiple-dd-same-pipeline/config.yaml",
		},
		{
			name:           "receivers/no-prometheus-receiver",
			provided:       "receivers/no-prometheus-receiver/config.yaml",
			expectedResult: "receivers/no-prometheus-receiver/config-result.yaml",
		},
		{
			name:           "receivers/no-prom-multi-dd",
			provided:       "receivers/no-prom-multi-dd/config.yaml",
			expectedResult: "receivers/no-prom-multi-dd/config-result.yaml",
		},
		{
			name:           "receivers/no-prom-not-default-addr",
			provided:       "receivers/no-prom-not-default-addr/config.yaml",
			expectedResult: "receivers/no-prom-not-default-addr/config-result.yaml",
		},
		{
			name:           "receivers/multi-dd-partial-prom",
			provided:       "receivers/multi-dd-partial-prom/config.yaml",
			expectedResult: "receivers/multi-dd-partial-prom/config-result.yaml",
		},
		{
			name:           "receivers/no-receivers-defined",
			provided:       "receivers/no-receivers-defined/config.yaml",
			expectedResult: "receivers/no-receivers-defined/config-result.yaml",
		},
		{
			name:           "receivers/empty-staticconfigs",
			provided:       "receivers/empty-staticconfigs/config.yaml",
			expectedResult: "receivers/empty-staticconfigs/config-result.yaml",
		},
		{
			name:           "receivers/missing-staticconfigs-section",
			provided:       "receivers/missing-staticconfigs-section/config.yaml",
			expectedResult: "receivers/missing-staticconfigs-section/config-result.yaml",
		},
		{
			name:           "processors/dd-connector",
			provided:       "processors/dd-connector/config.yaml",
			expectedResult: "processors/dd-connector/config-result.yaml",
		},
		{
			name:           "processors/dd-connector-multi-pipelines",
			provided:       "processors/dd-connector-multi-pipelines/config.yaml",
			expectedResult: "processors/dd-connector-multi-pipelines/config-result.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/empty-string",
			provided:       "dd-core-cfg/apikey/empty-string/config.yaml",
			expectedResult: "dd-core-cfg/apikey/empty-string/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/empty-string/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/unset",
			provided:       "dd-core-cfg/apikey/unset/config.yaml",
			expectedResult: "dd-core-cfg/apikey/unset/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/unset/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/unset-number",
			provided:       "dd-core-cfg/apikey/unset-number/config.yaml",
			expectedResult: "dd-core-cfg/apikey/unset-number/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/unset-number/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/secret",
			provided:       "dd-core-cfg/apikey/secret/config.yaml",
			expectedResult: "dd-core-cfg/apikey/secret/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/secret/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/api-set-no-key",
			provided:       "dd-core-cfg/apikey/api-set-no-key/config.yaml",
			expectedResult: "dd-core-cfg/apikey/api-set-no-key/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/api-set-no-key/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/no-api-key-section",
			provided:       "dd-core-cfg/apikey/no-api-key-section/config.yaml",
			expectedResult: "dd-core-cfg/apikey/no-api-key-section/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/no-api-key-section/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/apikey/multiple-dd-exporter",
			provided:       "dd-core-cfg/apikey/multiple-dd-exporter/config.yaml",
			expectedResult: "dd-core-cfg/apikey/multiple-dd-exporter/config-result.yaml",
			agentConfig:    "dd-core-cfg/apikey/multiple-dd-exporter/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/empty-string",
			provided:       "dd-core-cfg/site/empty-string/config.yaml",
			expectedResult: "dd-core-cfg/site/empty-string/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/empty-string/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/multiple-dd-exporter",
			provided:       "dd-core-cfg/site/multiple-dd-exporter/config.yaml",
			expectedResult: "dd-core-cfg/site/multiple-dd-exporter/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/multiple-dd-exporter/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/no-api-site-section",
			provided:       "dd-core-cfg/site/no-api-site-section/config.yaml",
			expectedResult: "dd-core-cfg/site/no-api-site-section/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/no-api-site-section/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/unset",
			provided:       "dd-core-cfg/site/unset/config.yaml",
			expectedResult: "dd-core-cfg/site/unset/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/unset/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/unset-core-mptystr-col",
			provided:       "dd-core-cfg/site/unset-core-mptystr-col/config.yaml",
			expectedResult: "dd-core-cfg/site/unset-core-mptystr-col/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/unset-core-mptystr-col/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/site/api-set-no-site",
			provided:       "dd-core-cfg/site/api-set-no-site/config.yaml",
			expectedResult: "dd-core-cfg/site/api-set-no-site/config-result.yaml",
			agentConfig:    "dd-core-cfg/site/api-set-no-site/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/all/no-overrides",
			provided:       "dd-core-cfg/all/no-overrides/config.yaml",
			expectedResult: "dd-core-cfg/all/no-overrides/config.yaml",
			agentConfig:    "dd-core-cfg/all/no-overrides/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/all/api-section",
			provided:       "dd-core-cfg/all/api-section/config.yaml",
			expectedResult: "dd-core-cfg/all/api-section/config-result.yaml",
			agentConfig:    "dd-core-cfg/all/api-section/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/all/key-site-section",
			provided:       "dd-core-cfg/all/key-site-section/config.yaml",
			expectedResult: "dd-core-cfg/all/key-site-section/config-result.yaml",
			agentConfig:    "dd-core-cfg/all/key-site-section/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/all/no-api-section",
			provided:       "dd-core-cfg/all/no-api-section/config.yaml",
			expectedResult: "dd-core-cfg/all/no-api-section/config-result.yaml",
			agentConfig:    "dd-core-cfg/all/no-api-section/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/none",
			provided:       "dd-core-cfg/none/config.yaml",
			expectedResult: "dd-core-cfg/none/config-result.yaml",
			agentConfig:    "dd-core-cfg/none/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/env/no-profiler-options",
			provided:       "dd-core-cfg/env/no-profiler-options/config.yaml",
			expectedResult: "dd-core-cfg/env/no-profiler-options/config-result.yaml",
			agentConfig:    "dd-core-cfg/env/no-profiler-options/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/env/no-env",
			provided:       "dd-core-cfg/env/no-env/config.yaml",
			expectedResult: "dd-core-cfg/env/no-env/config-result.yaml",
			agentConfig:    "dd-core-cfg/env/no-env/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/env/no-override",
			provided:       "dd-core-cfg/env/no-override/config.yaml",
			expectedResult: "dd-core-cfg/env/no-override/config-result.yaml",
			agentConfig:    "dd-core-cfg/env/no-override/acfg.yaml",
		},
		{
			name:           "dd-core-cfg/env/empty-profiler-options",
			provided:       "dd-core-cfg/env/empty-profiler-options/config.yaml",
			expectedResult: "dd-core-cfg/env/empty-profiler-options/config-result.yaml",
			agentConfig:    "dd-core-cfg/env/empty-profiler-options/acfg.yaml",
		},
		{
			name:           "features/all-features",
			provided:       "features/all-features/config.yaml",
			expectedResult: "features/all-features/config-result.yaml",
			agentConfig:    "features/all-features/acfg.yaml",
		},
		{
			name:           "features/all-extensions-only",
			provided:       "features/all-extensions-only/config.yaml",
			expectedResult: "features/all-extensions-only/config-result.yaml",
			agentConfig:    "features/all-extensions-only/acfg.yaml",
		},
		{
			name:           "features/some-extensions-only",
			provided:       "features/some-extensions-only/config.yaml",
			expectedResult: "features/some-extensions-only/config-result.yaml",
			agentConfig:    "features/some-extensions-only/acfg.yaml",
		},
		{
			name:           "features/infraattributes-only",
			provided:       "features/infraattributes-only/config.yaml",
			expectedResult: "features/infraattributes-only/config-result.yaml",
			agentConfig:    "features/infraattributes-only/acfg.yaml",
		},
		{
			name:           "features/no-features",
			provided:       "features/no-features/config.yaml",
			expectedResult: "features/no-features/config.yaml",
			agentConfig:    "features/no-features/acfg.yaml",
		},
		{
			name:           "features/prometheus-only",
			provided:       "features/prometheus-only/config.yaml",
			expectedResult: "features/prometheus-only/config-result.yaml",
			agentConfig:    "features/prometheus-only/acfg.yaml",
		},
		{
			name:           "features/no-defined-features",
			provided:       "features/no-defined-features/config.yaml",
			expectedResult: "features/no-defined-features/config-result.yaml",
			agentConfig:    "features/no-defined-features/acfg.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Requires{}
			if tc.agentConfig != "" {
				f, err := os.ReadFile(uriFromFile(tc.agentConfig)[0])
				require.NoError(t, err)
				acfg := config.NewMockFromYAML(t, string(f))
				r.Conf = acfg
			}
			converter, err := NewConverterForAgent(r)
			assert.NoError(t, err)

			resolver, err := newResolver(uriFromFile(tc.provided))
			assert.NoError(t, err)
			conf, err := resolver.Resolve(context.Background())
			assert.NoError(t, err)

			converter.Convert(context.Background(), conf)

			resolverResult, err := newResolver(uriFromFile(tc.expectedResult))
			assert.NoError(t, err)
			confResult, err := resolverResult.Resolve(context.Background())
			assert.NoError(t, err)

			assert.Equal(t, confResult.ToStringMap(), conf.ToStringMap())
		})
	}

	// test using newConverter function to simulate ocb environment
	nopLogger := zap.NewNop()
	for _, tc := range tests {
		if tc.agentConfig != "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			converter := newConverter(confmap.ConverterSettings{Logger: nopLogger})

			resolver, err := newResolver(uriFromFile(tc.provided))
			assert.NoError(t, err)
			conf, err := resolver.Resolve(context.Background())
			assert.NoError(t, err)

			converter.Convert(context.Background(), conf)

			resolverResult, err := newResolver(uriFromFile(tc.expectedResult))
			assert.NoError(t, err)
			confResult, err := resolverResult.Resolve(context.Background())
			assert.NoError(t, err)

			assert.Equal(t, confResult.ToStringMap(), conf.ToStringMap())
		})
	}
}

func TestConvert_APIKeyFromEnvVar(t *testing.T) {
	t.Setenv("DD_API_KEY", "123456")
	t.Setenv("DD_SITE", "")
	converter, err := NewConverterForAgent(Requires{config.NewMock(t)})
	assert.NoError(t, err)

	resolver, err := newResolver(uriFromFile("dd-core-cfg/apikey/unset-number/config.yaml"))
	assert.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	assert.NoError(t, err)

	converter.Convert(context.Background(), conf)

	resolverResult, err := newResolver(uriFromFile("dd-core-cfg/apikey/unset-number/config-result.yaml"))
	assert.NoError(t, err)
	confResult, err := resolverResult.Resolve(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, confResult.ToStringMap(), conf.ToStringMap())
}
