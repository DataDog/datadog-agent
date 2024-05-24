// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package provider

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v3"
)

// ExtendedConfigProvider implements the otelcol.ConfigProvider interface and
// provides extra functions to expose the provided and enhanced configs.
type ExtendedConfigProvider interface {
	otelcol.ConfigProvider
	GetProvidedConf() string
	GetEnhancedConf() string
}

type configProvider struct {
	base     otelcol.ConfigProvider
	confDump confDump
}

type confDump struct {
	provided string
	enhanced string
}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

// currently only supports a single URI in the uris slice, and this URI needs to be a file path.
func NewConfigProvider(uris []string) (ExtendedConfigProvider, error) {
	ocp, err := otelcol.NewConfigProvider(newDefaultConfigProviderSettings(uris))
	if err != nil {
		return nil, fmt.Errorf("failed to create configprovider: %w", err)
	}

	// this is a hack until we are unblocked from upstream to be able to use confToString.
	yamlBytes, err := os.ReadFile(uris[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return &configProvider{
		base: ocp,
		confDump: confDump{
			provided: string(yamlBytes),
			enhanced: "not supported",
		},
	}, nil
}

func newDefaultConfigProviderSettings(uris []string) otelcol.ConfigProviderSettings {
	return otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: uris,
			ProviderFactories: []confmap.ProviderFactory{
				fileprovider.NewFactory(),
				envprovider.NewFactory(),
				yamlprovider.NewFactory(),
				httpprovider.NewFactory(),
				httpsprovider.NewFactory(),
			},
			ConverterFactories: []confmap.ConverterFactory{expandconverter.NewFactory()},
		},
	}
}

func (cp *configProvider) Get(ctx context.Context, factories otelcol.Factories) (*otelcol.Config, error) {
	conf, err := cp.base.Get(ctx, factories)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// err = cp.addProvidedConf(conf)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to add provided conf: %w", err)
	// }

	//
	// TODO: modify conf (add datadogconnector if not present ...etc)
	//

	// err = cp.addEnhancedConf(conf)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to add enhanced conf: %w", err)
	// }

	return conf, nil
}

func (cp *configProvider) addProvidedConf(conf *otelcol.Config) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	cp.confDump.provided = bytesConf
	return nil
}

func (cp *configProvider) addEnhancedConf(conf *otelcol.Config) error {
	bytesConf, err := confToString(conf)
	if err != nil {
		return err
	}

	cp.confDump.enhanced = bytesConf
	return nil
}

// GetProvidedConf returns a string representing the collector configuration passed
// by the user. Should not be called concurrently with Get.
// Note: the current implementation does not redact sensitive data (e.g. API Key). 
// Once we are unblocked and are able to remove the hack, this will provide the config 
// with any sensitive data redacted.
func (cp *configProvider) GetProvidedConf() string {
	return cp.confDump.provided
}

// GetEnhancedConf returns a string representing the ehnhanced collector configuration.
// Should not be called concurrently with Get.
// Note: this is currently not supported. 
func (cp *configProvider) GetEnhancedConf() string {
	return cp.confDump.enhanced
}

// confToString takes in an otelcol.Config and returns a string with the yaml
// representation. It takes advantage of the confmaps opaquevalue to redact any
// sensitive fields.
// Note: Currently not supported until the following upstream PR:
// https://github.com/open-telemetry/opentelemetry-collector/pull/10139 is merged.
func confToString(conf *otelcol.Config) (string, error) {
	cfg := confmap.New()
	err := cfg.Marshal(conf)
	if err != nil {
		return "", err
	}

	bytesConf, err := yaml.Marshal(cfg.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
}

// Watch is a no-op which returns a nil chan.
func (cp *configProvider) Watch() <-chan error {
	return nil
}

func (cp *configProvider) Shutdown(ctx context.Context) error {
	return cp.base.Shutdown(ctx)
}
