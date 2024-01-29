// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Program otlp_sender sends telemetry data defined in a given file
package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/loggingexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/tests/e2e/containers/otlp_sender/internal/filereceiver"
)

func components() (
	otelcol.Factories,
	error,
) {
	var errs error

	extensions, err := extension.MakeFactoryMap()
	errs = multierr.Append(errs, err)

	receivers, err := receiver.MakeFactoryMap(
		filereceiver.NewFactory(),
	)
	errs = multierr.Append(errs, err)

	exporters, err := exporter.MakeFactoryMap(
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
		loggingexporter.NewFactory(),
	)
	errs = multierr.Append(errs, err)

	processors, err := processor.MakeFactoryMap()
	errs = multierr.Append(errs, err)

	factories := otelcol.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}

	return factories, errs
}

func main() {
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	cmd := otelcol.NewCommand(otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Command:     "otlpsender",
			Description: "OpenTelemetry test sender",
			Version:     "latest",
		},
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
	})
	if err := cmd.Execute(); err != nil {
		log.Fatalf("collector server run finished with error: %v", err)
	}
}
