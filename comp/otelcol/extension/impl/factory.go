package impl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
)

const (
	defaultHTTPPort = 7777
)

type ddExtensionFactory struct {
	extension.Factory

	provider otelcol.ConfigProvider
}

// NewFactory creates a factory for HealthCheck extension.
func NewFactory(provider otelcol.ConfigProvider) extension.Factory {
	return &ddExtensionFactory{
		provider: provider,
	}
}

func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {

	config := &Config{
		Provider: f.provider,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return newDDHTTPExtension(ctx, config, set.TelemetrySettings, set.BuildInfo)
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: fmt.Sprintf("localhost:%d", defaultHTTPPort),
		},
		Provider: f.provider,
	}
}

func (f *ddExtensionFactory) Type() component.Type {
	return metadata.Type
}

func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}
