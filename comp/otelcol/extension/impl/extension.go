package impl

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

var Type = metadata.Type

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg *Config // Extension configuration.

	telemetry component.TelemetrySettings
	server    *http.Server
	info      component.BuildInfo
}

// newDDHTTPExtension creates a new instance of the extension.
func newDDHTTPExtension(ctx context.Context, cfg *Config, telemetry component.TelemetrySettings, info component.BuildInfo) (extension.Extension, error) {
	ext := &ddExtension{
		cfg:       cfg,
		telemetry: telemetry,
		info:      info,
		server: &http.Server{
			Addr: cfg.HTTPConfig.Endpoint,
		},
	}

	ext.server.Handler = ext

	return ext, nil
}

// Start is called when the extension is started.
func (ext *ddExtension) Start(ctx context.Context, host component.Host) error {
	ext.telemetry.Logger.Info("Starting DD Extension HTTP server", zap.String("url", ext.cfg.HTTPConfig.Endpoint))

	// Start the server in a goroutine.
	go func() {
		if err := ext.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ext.telemetry.ReportStatus(component.NewFatalErrorEvent(err))
		} else {
			ext.telemetry.Logger.Info("DD Extension HTTP server started successfully at", zap.String("url", ext.cfg.HTTPConfig.Endpoint))
		}
	}()

	// List configured Extensions
	extensions := host.GetExtensions()
	for extension, _ := range extensions {
		ext.telemetry.Logger.Info("Extension available", zap.String("extension", extension.String()))
	}

	return nil
}

// Shutdown is called when the extension is shut down.
func (ext *ddExtension) Shutdown(ctx context.Context) error {
	// Clean up any resources used by the extension
	ext.telemetry.Logger.Info("Shutting down HTTP server")

	// Give the server a grace period to finish handling requests.
	return ext.server.Shutdown(ctx)
}

// Start is called when the extension is started.
func (ext *ddExtension) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from my OpenTelemetry extension!\n")
}
