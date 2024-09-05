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
	"net"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"

	extensionDef "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
)

// Type exports the internal metadata type for easy reference
var Type = metadata.Type

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	extension.Extension // Embed base Extension for common functionality.

	cfg *Config // Extension configuration.

	telemetry   component.TelemetrySettings
	server      *http.Server
	tlsListener net.Listener
	info        component.BuildInfo
	debug       extensionDef.DebugSourceResponse
}

var _ extension.Extension = (*ddExtension)(nil)

// NewExtension creates a new instance of the extension.
func NewExtension(_ context.Context, cfg *Config, telemetry component.TelemetrySettings, info component.BuildInfo) (extensionDef.Component, error) {
	ext := &ddExtension{
		cfg:       cfg,
		telemetry: telemetry,
		info:      info,
		debug: extensionDef.DebugSourceResponse{
			Sources: map[string]extensionDef.OTelFlareSource{},
		},
	}

	var err error
	ext.server, ext.tlsListener, err = buildHTTPServer(cfg.HTTPConfig.Endpoint, ext)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

// Start is called when the extension is started.
func (ext *ddExtension) Start(_ context.Context, host component.Host) error {
	ext.telemetry.Logger.Info("Starting DD Extension HTTP server", zap.String("url", ext.cfg.HTTPConfig.Endpoint))

	// List configured Extensions
	configstore := ext.cfg.ConfigStore
	c, err := configstore.GetEnhancedConf()
	if err != nil {
		return err
	}

	extensionConfs, err := c.Sub("extensions")
	if err != nil {
		return nil
	}

	extensions := host.GetExtensions()
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

	go func() {
		if err := ext.server.Serve(ext.tlsListener); err != nil && err != http.ErrServerClosed {
			ext.telemetry.ReportStatus(component.NewFatalErrorEvent(err))
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
	return ext.server.Shutdown(ctx)
}

// ServeHTTP the request handler for the extension.
func (ext *ddExtension) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	customer, err := ext.cfg.ConfigStore.GetProvidedConfAsString()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to get provided config\n")
		return
	}
	enhanced, err := ext.cfg.ConfigStore.GetEnhancedConfAsString()
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
			AgentVersion:     ext.info.Version,
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
