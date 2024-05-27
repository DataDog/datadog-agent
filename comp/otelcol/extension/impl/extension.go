package impl

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
)

// DDExtension is a basic OpenTelemetry Collector extension.
type DDExtension struct {
	// Configuration options for your extension
}

// Factory creates a new MyExtension instance.
func Factory() component.Extension {
	return &DDExtension{}
}

// Start is called when the extension is started.
func (ext *DDExtension) Start(ctx context.Context, params component.StartParams) error {
	// Implement your extension logic here
	// This could involve starting background tasks, registering for callbacks, etc.
	return nil
}

// Shutdown is called when the extension is shut down.
func (ext *DDExtension) Shutdown(ctx context.Context) error {
	// Clean up any resources used by the extension
	return nil
}

// Capabilities describes the capabilities of the extension.
func (ext *DDExtension) Capabilities() config.ComponentCapabilities {
	return config.ComponentCapabilities{
		SupportedDataType: []string{ // List of supported data types (e.g., traces, metrics)
		},
	}
}
