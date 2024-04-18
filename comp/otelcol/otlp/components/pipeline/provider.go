package pipeline

import (
	"context"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

type configProvider struct {
	uris []string
}

var _ otelcol.ConfigProvider = (*configProvider)(nil)

func NewProvider(uris []string) otelcol.ConfigProvider {
	return &configProvider{uris: uris}
}

func (p *configProvider) Get(ctx context.Context, factories otelcol.Factories) (*otelcol.Config, error) {
	config, err := loadConfig(ctx, p.uris, factories)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// Watch blocks until any configuration change was detected or an unrecoverable error
// happened during monitoring the configuration changes.
//
// Error is nil if the configuration is changed and needs to be re-fetched. Any non-nil
// error indicates that there was a problem with watching the config changes.
//
// Should never be called concurrently with itself or Get.
func (p *configProvider) Watch() <-chan error {
	ch := make(chan error)
	return ch
}

// Shutdown signals that the provider is no longer in use and the that should close
// and release any resources that it may have created.
//
// This function must terminate the Watch channel.
//
// Should never be called concurrently with itself or Get.
func (p *configProvider) Shutdown(ctx context.Context) error {
	return nil
}

// loadConfig loads a config.Config
func loadConfig(ctx context.Context, uris []string, factories otelcol.Factories) (*otelcol.Config, error) {
	// Read yaml config from file
	set := confmap.ProviderSettings{}
	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: uris,
			Providers: makeMapProvidersMap(
				fileprovider.NewWithSettings(set),
				envprovider.NewWithSettings(set),
				yamlprovider.NewWithSettings(set),
				httpprovider.NewWithSettings(set),
			),
			Converters: []confmap.Converter{expandconverter.New(confmap.ConverterSettings{})},
		},
	})
	if err != nil {
		return nil, err
	}
	cfg, err := provider.Get(ctx, factories)
	if err != nil {
		return nil, err
	}
	err = cfg.Validate()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func makeMapProvidersMap(providers ...confmap.Provider) map[string]confmap.Provider {
	ret := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		ret[provider.Scheme()] = provider
	}
	return ret
}
