package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	provider "github.com/DataDog/datadog-agent/comp/otelcol/provider/def"
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
	provider, ok := ext.cfg.Provider.(provider.Component)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to get config provider\n")
		return
	}
	resp := Response{
		BuildInfoResponse: BuildInfoResponse{
			agentVersion: ext.info.Version,
			agentCommand: ext.info.Command,
			agentDesc:    ext.info.Description,
		},
		ConfigResponse: ConfigResponse{
			customerConfig: provider.GetProvidedConf(),
			runtimeConfig:  provider.GetEnhancedConf(),
		},
		DebugSourceResponse: DebugSourceResponse{},
	}
	ext.telemetry.Logger.Info("Logging response", zap.String("response", fmt.Sprintf("%v", resp)))

	j, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to marshal output: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, string(j))

}
