// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/zap"

	extensionDef "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Type exports the internal metadata type for easy reference
var Type = metadata.Type

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg *Config // Extension configuration.

	telemetry   component.TelemetrySettings
	server      *server
	info        component.BuildInfo
	debug       extensionDef.DebugSourceResponse
	configStore *configStore
}

var _ extension.Extension = (*ddExtension)(nil)

// NotifyConfig implements the ConfigWatcher interface, which allows this extension
// to be notified of the Collector's effective configuration. See interface:
// https://github.com/open-telemetry/opentelemetry-collector/blob/d0fde2f6b98f13cbbd8657f8188207ac7d230ed5/extension/extension.go#L46.

// This method is called during the startup process by the Collector's Service right after
// calling Start.
func (ext *ddExtension) NotifyConfig(_ context.Context, conf *confmap.Conf) error {
	var cfg *configSettings
	var err error

	if cfg, err = unmarshal(conf, *ext.cfg.factories); err != nil {
		return fmt.Errorf("cannot unmarshal the configuration: %w", err)
	}

	config := &otelcol.Config{
		Receivers:  cfg.Receivers.Configs(),
		Processors: cfg.Processors.Configs(),
		Exporters:  cfg.Exporters.Configs(),
		Connectors: cfg.Connectors.Configs(),
		Extensions: cfg.Extensions.Configs(),
		Service:    cfg.Service,
	}

	ext.configStore.setEnhancedConf(config)

	// List configured Extensions
	c, err := ext.configStore.getEnhancedConf()
	if err != nil {
		return err
	}

	extensionConfs, err := c.Sub("extensions")
	if err != nil {
		return nil
	}

	extensions := config.Extensions
	for extension := range extensions {
		extractor, ok := supportedDebugExtensions[extension.Type().String()]
		if !ok {
			continue
		}

		exconf, err := extensionConfs.Sub(extension.String())
		if err != nil {
			ext.telemetry.Logger.Info("There was an issue pulling the configuration for", zap.String("extension", extension.String()))
			continue
		}

		uri, err := extractor(exconf)

		var uris []string
		switch extension.Type().String() {
		case "pprof":
			uris = []string{
				uri + "/debug/pprof/heap",
				uri + "/debug/pprof/allocs",
				uri + "/debug/pprof/profile",
			}
		case "zpages":
			uris = []string{
				uri + "/debug/servicez",
				uri + "/debug/pipelinez",
				uri + "/debug/extensionz",
				uri + "/debug/featurez",
				uri + "/debug/tracez",
			}
		default:
			uris = []string{uri}
		}

		if err != nil {
			ext.telemetry.Logger.Info("Unavailable debug extension for", zap.String("extension", extension.String()))
			continue
		}

		ext.telemetry.Logger.Info("Found debug extension at", zap.String("uri", uri))
		ext.debug.Sources[extension.String()] = extensionDef.OTelFlareSource{
			URLs: uris,
		}
	}

	return nil
}

// NewExtension creates a new instance of the extension.
func NewExtension(_ context.Context, cfg *Config, telemetry component.TelemetrySettings, info component.BuildInfo) (extensionDef.Component, error) {
	ocpProvided, err := otelcol.NewConfigProvider(cfg.configProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to create configprovider: %w", err)
	}

	providedConf, err := ocpProvided.Get(context.Background(), *cfg.factories)
	if err != nil {
		return nil, err
	}

	ext := &ddExtension{
		cfg:         cfg,
		telemetry:   telemetry,
		info:        info,
		configStore: &configStore{},
		debug: extensionDef.DebugSourceResponse{
			Sources: map[string]extensionDef.OTelFlareSource{},
		},
	}

	ext.configStore.setProvidedConf(providedConf)

	ext.server, err = newServer(cfg.HTTPConfig.Endpoint, ext)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

// Start is called when the extension is started.
func (ext *ddExtension) Start(_ context.Context, host component.Host) error {
	ext.telemetry.Logger.Info("Starting DD Extension HTTP server", zap.String("url", ext.cfg.HTTPConfig.Endpoint))

	go func() {
		if err := ext.server.start(); err != nil && err != http.ErrServerClosed {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
			ext.telemetry.Logger.Info("DD Extension HTTP could not start", zap.String("err", err.Error()))
		}
	}()

	return nil
}

// Shutdown is called when the extension is shut down.
func (ext *ddExtension) Shutdown(ctx context.Context) error {
	// Clean up any resources used by the extension
	ext.telemetry.Logger.Info("Shutting down HTTP server")

	// Give the server a grace period to finish handling requests.
	return ext.server.shutdown(ctx)
}

// ServeHTTP the request handler for the extension.
func (ext *ddExtension) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	customer, err := ext.configStore.getProvidedConfAsString()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to get provided config\n")
		return
	}
	enhanced, err := ext.configStore.getEnhancedConfAsString()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to get enhanced config\n")
		return
	}

	envconfig := ""
	envvars := getEnvironmentAsMap()
	if envbytes, err := json.Marshal(envvars); err == nil {
		envconfig = string(envbytes)
	}

	resp := extensionDef.Response{
		BuildInfoResponse: extensionDef.BuildInfoResponse{
			AgentVersion:     version.AgentVersion,
			AgentCommand:     ext.info.Command,
			AgentDesc:        ext.info.Description,
			ExtensionVersion: ext.info.Version,
		},
		ConfigResponse: extensionDef.ConfigResponse{
			CustomerConfig:        customer,
			RuntimeConfig:         enhanced,
			RuntimeOverrideConfig: "", // TODO: support RemoteConfig
			EnvConfig:             envconfig,
		},
		DebugSourceResponse: ext.debug,
		Environment:         envvars,
	}

	j, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to marshal output: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(j))

}
