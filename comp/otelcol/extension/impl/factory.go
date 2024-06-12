package impl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
)

const (
	defaultHTTPPort = 7777
)

type ddExtensionFactory struct {
	extension.Factory

	converter confmap.Converter
}

// NewFactory creates a factory for HealthCheck extension.
func NewFactory(converter confmap.Converter) extension.Factory {
	return &ddExtensionFactory{
		converter: converter,
	}
}

func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {

	config := &Config{
		Converter: f.converter,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return newDDHTTPExtension(ctx, config, set.TelemetrySettings, set.BuildInfo)
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: fmt.Sprintf("localhost:%d", defaultHTTPPort),
		},
		Converter: f.converter,
	}
}

func (f *ddExtensionFactory) Type() component.Type {
	return metadata.Type
}

func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}
