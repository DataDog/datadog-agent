// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	logsmapping "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs"
	"github.com/stormcat24/protodep/pkg/logger"
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
	translator, err := logsmapping.NewTranslator(set, attributesTranslator, otelSource)
	if err != nil {
		return nil, err
	}

	return &exporter{
		set:              set,
		logsAgentChannel: logsAgentChannel,
		logSource:        logSource,
		translator:       translator,
	}, nil
}

func (e *exporter) ConsumeLogs(ctx context.Context, ld plog.Logs) (err error) {
	defer func() {
		if err != nil {
			newErr, scrubbingErr := scrubber.ScrubString(err.Error())
			if scrubbingErr != nil {
				err = scrubbingErr
			} else {
				err = errors.New(newErr)
			}
		}
	}()

	payloads := e.translator.MapLogs(ctx, ld)
	for _, ddLog := range payloads {
		tags := strings.Split(ddLog.GetDdtags(), ",")
		// Tags are set in the message origin instead
		ddLog.Ddtags = nil
		service := ""
		if ddLog.Service != nil {
			service = *ddLog.Service
		}
		status := ddLog.AdditionalProperties["status"]
		if status == "" {
			status = message.StatusInfo
		}
		origin := message.NewOrigin(e.logSource)
		origin.SetTags(tags)
		origin.SetService(service)
		origin.SetSource(logSourceName)

		content, err := ddLog.MarshalJSON()
		if err != nil {
			logger.Error("Error parsing log: " + err.Error())
		}

		// ingestionTs is an internal field used for latency tracking on the status page, not the actual log timestamp.
		ingestionTs := time.Now().UnixNano()
		message := message.NewMessage(content, origin, status, ingestionTs)

		e.logsAgentChannel <- message
	}

	return nil
}
