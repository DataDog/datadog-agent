// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package provider

import (
	"context"
	"fmt"

	"github.com/mitchellh/mapstructure"
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

type ExtendedConfigProvider interface {
	otelcol.ConfigProvider
	GetProvidedConf() []byte
	GetEnhancedConf() []byte
}

type configProvider struct {
	base     otelcol.ConfigProvider
	confDump confDump
}

type confDump struct {
	provided []byte
	enhanced []byte
}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

func NewConfigProvider(uris []string) (ExtendedConfigProvider, error) {
	ocp, err := otelcol.NewConfigProvider(newDefaultConfigProviderSettings(uris))
	if err != nil {
		return nil, fmt.Errorf("failed to create configprovider: %w", err)
	}

	return &configProvider{
		base: ocp,
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

	err = cp.addProvidedConf(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to add provided conf: %w", err)
	}

	//
	// TODO: modify conf (add datadogconnector if not present ...etc)
	//

	err = cp.addEnhancedConf(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to add enhanced conf: %w", err)
	}

	return conf, nil
}

func (cp *configProvider) addProvidedConf(conf *otelcol.Config) error {
	bytesConf, err := confToBytes(conf)
	if err != nil {
		return err
	}

	cp.confDump.provided = bytesConf
	return nil
}

func (cp *configProvider) addEnhancedConf(conf *otelcol.Config) error {
	bytesConf, err := confToBytes(conf)
	if err != nil {
		return err
	}

	cp.confDump.enhanced = bytesConf
	return nil
}

// GetProvidedConf returns a []byte representing the collector configuration passed
// by the user. Should not be called concurrently with Get.
func (cp *configProvider) GetProvidedConf() []byte {
	return cp.confDump.provided
}

// GetEnhancedConf returns a []byte representing the ehnhanced collector configuration.
// Should not be called concurrently with Get.
func (cp *configProvider) GetEnhancedConf() []byte {
	return cp.confDump.enhanced
}

func confToBytes(conf *otelcol.Config) ([]byte, error) {
	stringMap := map[string]interface{}{}
	if err := mapstructure.Decode(conf, &stringMap); err != nil {
		return nil, err
	}

	bytesConf, err := yaml.Marshal(stringMap)
	if err != nil {
		return nil, err
	}
	return bytesConf, nil
}

// Watch is a no-op which returns a nil chan.
func (cp *configProvider) Watch() <-chan error {
	return nil
}

func (cp *configProvider) Shutdown(ctx context.Context) error {
	return cp.base.Shutdown(ctx)
}
