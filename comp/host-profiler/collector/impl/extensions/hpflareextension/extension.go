// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextension defines the OpenTelemetry Extension implementation.
package hpflareextension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/goccy/go-yaml"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/extension/extensioncapabilities"
	"go.uber.org/zap"
)

var _ extensioncapabilities.ConfigWatcher = (*DDExtension)(nil)
var _ http.Handler = (*DDExtension)(nil)

// Response is the response struct for API queries
type Response struct {
	Config string `json:"config"`
}

// DDExtension is a basic OpenTelemetry Collector extension.
type DDExtension struct {
	cfg       *Config
	config    string
	telemetry component.TelemetrySettings
	server    *server
}

// NotifyConfig implements the ConfigWatcher interface, which allows this extension
// to be notified of the Collector's effective configuration. See interface:
// https://github.com/open-telemetry/opentelemetry-collector/blob/d0fde2f6b98f13cbbd8657f8188207ac7d230ed5/extension/extension.go#L46.
// This method is called during the startup process by the Collector's Service right after
// calling Start.
func (ext *DDExtension) NotifyConfig(_ context.Context, conf *confmap.Conf) error {
	if conf == nil {
		msg := "received a nil config in ddExtension.NotifyConfig"
		return errors.New(msg)
	}
	var err error
	confMap := conf.ToStringMap()
	enhancedBytes, err := yaml.Marshal(confMap)
	if err != nil {
		return err
	}
	ext.config = string(enhancedBytes)
	return nil
}

// NewExtension creates a new instance of the extension.
func NewExtension(cfg *Config, ipcComp ipc.Component, telemetry component.TelemetrySettings) (*DDExtension, error) {
	var err error
	ext := &DDExtension{
		cfg:       cfg,
		telemetry: telemetry,
	}
	ext.server, err = newServer(cfg.HTTPConfig.NetAddr.Endpoint, ext, ipcComp)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

func (ext *DDExtension) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	j, err := json.MarshalIndent(Response{
		Config: ext.config,
	}, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to marshal output: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(j))
}

// Start is called when the extension is started.
func (ext *DDExtension) Start(_ context.Context, host component.Host) error {
	ext.telemetry.Logger.Info("Starting DD Extension HTTP server", zap.String("url", ext.cfg.HTTPConfig.NetAddr.Endpoint))

	go func() {
		if err := ext.server.start(); err != nil && err != http.ErrServerClosed {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
			ext.telemetry.Logger.Info("DD Extension HTTP could not start", zap.String("err", err.Error()))
		}
	}()

	return nil
}

// Shutdown is called when the extension is shut down.
func (ext *DDExtension) Shutdown(ctx context.Context) error {
	return ext.server.shutdown(ctx)
}
