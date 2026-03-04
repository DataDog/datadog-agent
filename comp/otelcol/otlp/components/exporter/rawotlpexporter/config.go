// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rawotlpexporter provides an OpenTelemetry Collector exporter that
// sends OTLP trace payloads as raw bytes to the trace agent's RawTraceService.
package rawotlpexporter

import (
	"errors"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config defines configuration for the raw OTLP trace exporter that sends
// serialized OTLP ExportTraceServiceRequest bytes to the trace agent's
// RawTraceService.ExportTracesRaw endpoint (efficient path).
type Config struct {
	// Endpoint is the host:port of the trace agent's gRPC server (e.g. "localhost:4317").
	Endpoint string `mapstructure:"endpoint"`
	// TLSSetting enables or disables TLS. For localhost loopback, Insecure is typically true.
	TLSSetting struct {
		Insecure bool `mapstructure:"insecure"`
	} `mapstructure:"tls"`
	// QueueSettings allows configuring the sending queue (optional; mapstructure key is "sending_queue").
	QueueSettings configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	return nil
}
