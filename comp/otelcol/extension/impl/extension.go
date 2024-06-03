package impl

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

const extensionName = "dd_extension"

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg *Config // Extension configuration.

	telemetry component.TelemetrySettings
	server    *http.Server
}

// Create the extension instance
func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newMyHTTPExtension(cfg.(*Config), set.Logger, set.TelemetrySettings)
}

// newMyHTTPExtension creates a new instance of the extension.
func newMyHTTPExtension(cfg *Config, telemetry component.TelemetrySettings) (extension.Extension, error) {
	return &ddExtension{
		cfg:       cfg,
		telemetry: telemetry,
		server: &http.Server{
			Addr: fmt.Sprintf(":%d", cfg.Port),
		},
	}, nil
}

// Start is called when the extension is started.
func (ext *ddExtension) Start(ctx context.Context, params component.StartParams) error {
	// Implement your extension logic here
	ext.logger.Info("Starting HTTP server", zap.Int("port", ext.cfg.Port))

	// Define your HTTP server handlers (e.g., /metrics, /health).
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from my OpenTelemetry extension!")
	})

	// Start the server in a goroutine.
	go func() {
		if err := ext.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ext.settings.telemetrySettings.ReportStatus(component.NewFatalErrorEvent(err))
		}
	}()

	return nil
}

// Shutdown is called when the extension is shut down.
func (ext *ddExtension) Shutdown(ctx context.Context) error {
	// Clean up any resources used by the extension
	ext.logger.Info("Shutting down HTTP server")

	// Give the server a grace period to finish handling requests.
	return ext.server.Shutdown(ctx)
}

// Capabilities describes the capabilities of the extension.
func (ext *ddExtension) Capabilities() config.ComponentCapabilities {
	return config.ComponentCapabilities{
		SupportedDataType: []string{ // List of supported data types (e.g., traces, metrics)
		},
	}
}
