package impl

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"

	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/localhostgate"
)

const (
	defaultHTTPPort = 7777
)

// NewFactory creates a factory for HealthCheck extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		metadata.Type,
		createDefaultConfig,
		createExtension,
		metadata.ExtensionStability,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: localhostgate.EndpointForPort(defaultHTTPPort),
		},
	}
}

// Create the extension instance
func createExtension(ctx context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newDDHTTPExtension(ctx, cfg.(*Config), set.TelemetrySettings), nil
}
