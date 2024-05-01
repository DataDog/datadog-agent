package provider

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

type configProvider struct {
	base otelcol.ConfigProvider
}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

func NewConfigProvider(uris []string) (otelcol.ConfigProvider, error) {
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
	//
	// TODO: modify conf (add datadogconnector if not present ...etc)
	//
	return conf, nil
}

// Watch is a no-op which returns a nil chan.
func (cp *configProvider) Watch() <-chan error {
	return nil
}

func (cp *configProvider) Shutdown(ctx context.Context) error {
	return cp.base.Shutdown(ctx)
}
