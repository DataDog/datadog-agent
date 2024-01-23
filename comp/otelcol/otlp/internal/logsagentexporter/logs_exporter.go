// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	logsmapping "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
)

// otelSource specifies a source to be added to all logs sent from the Datadog Agent.
// The tag has key `otel_source` and the value specified on this constant.
const otelSource = "datadog_agent"

type exporter struct {
	set              component.TelemetrySettings
	logsAgentChannel chan *message.Message
	logSource        *sources.LogSource
	translator       *logsmapping.Translator
}

func newExporter(
	set component.TelemetrySettings,
	logSource *sources.LogSource,
	logsAgentChannel chan *message.Message,
	attributesTranslator *attributes.Translator,
) (*exporter, error) {
	panic("not called")
}

func (e *exporter) ConsumeLogs(ctx context.Context, ld plog.Logs) (err error) {
	panic("not called")
}
