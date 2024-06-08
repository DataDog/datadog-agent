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
	debug     DebugSourceResponse
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
		debug: DebugSourceResponse{
			Sources: map[string]OTelFlareSource{},
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
	provider := ext.cfg.Provider.(provider.Component)
	c := provider.GetProvidedConf()
	extensionConfs, err := c.Sub("extensions")
	if err != nil {
		return nil
	}

	extensions := host.GetExtensions()
	for extension, _ := range extensions {
		extractor, ok := supportedDebugExtensions[extension.String()]
		if !ok {
			continue
		}

		exconf, err := extensionConfs.Sub(extension.String())
		if err != nil {
			ext.telemetry.Logger.Info("There was an issue pulling the configuration for", zap.String("extension", extension.String()))
			continue
		}

		uri, crawl := extractor(exconf)
		ext.telemetry.Logger.Info("Found debug extension at", zap.String("uri", uri))
		ext.debug.Sources[extension.String()] = OTelFlareSource{
			Url:   uri,
			Crawl: crawl,
		}
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

	customer, _ := provider.GetProvidedConfAsString()
	enhanced, _ := provider.GetEnhancedConfAsString()

	resp := Response{
		BuildInfoResponse{
			AgentVersion: ext.info.Version,
			AgentCommand: ext.info.Command,
			AgentDesc:    ext.info.Description,
		},
		ConfigResponse{
			CustomerConfig: customer,
			RuntimeConfig:  enhanced,
		},
		ext.debug,
		getEnvironmentAsMap(),
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
