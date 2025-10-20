// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextensionimpl defines the OpenTelemetry Extension implementation.
package hpflareextensionimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	extensionDef "github.com/DataDog/datadog-agent/comp/host-profiler/hpflareextension/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/goccy/go-yaml"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
)

type Response struct {
	Config string `json:"config"`
}

// ddExtension is a basic OpenTelemetry Collector extension.
type ddExtension struct {
	cfg       *Config
	config    string
	telemetry component.TelemetrySettings
	server    *server
}

func (ext *ddExtension) NotifyConfig(_ context.Context, conf *confmap.Conf) error {
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

func NewExtension(cfg *Config, ipcComp option.Option[ipc.Component], telemetry component.TelemetrySettings) (extensionDef.Component, error) {
	var err error
	ext := &ddExtension{
		cfg:       cfg,
		telemetry: telemetry,
	}
	ext.server, err = newServer(cfg.HTTPConfig.Endpoint, ext, ipcComp)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

func (ext *ddExtension) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
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
	return ext.server.shutdown(ctx)
}
