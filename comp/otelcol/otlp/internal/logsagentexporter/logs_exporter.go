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
	"go.uber.org/zap"

	logsmapping "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs"
	"go.opentelemetry.io/collector/pdata/plog"
)

// otelTag specifies a tag to be added to all logs sent from the Datadog Agent
const otelTag = "otel_source:datadog_agent"

// createConsumeLogsFunc returns an implementation of consumer.ConsumeLogsFunc
func createConsumeLogsFunc(logger *zap.Logger, logSource *sources.LogSource, logsAgentChannel chan *message.Message) func(context.Context, plog.Logs) error {

	return func(_ context.Context, ld plog.Logs) (err error) {
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

		rsl := ld.ResourceLogs()
		// Iterate over resource logs
		for i := 0; i < rsl.Len(); i++ {
			rl := rsl.At(i)
			sls := rl.ScopeLogs()
			res := rl.Resource()
			for j := 0; j < sls.Len(); j++ {
				sl := sls.At(j)
				lsl := sl.LogRecords()
				// iterate over Logs
				for k := 0; k < lsl.Len(); k++ {
					log := lsl.At(k)
					ddLog := logsmapping.Transform(log, res, logger)

					var tags []string
					if ddTags := ddLog.GetDdtags(); ddTags == "" {
						tags = []string{otelTag}
					} else {
						tags = append(strings.Split(ddTags, ","), otelTag)
					}
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
					origin := message.NewOrigin(logSource)
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

					logsAgentChannel <- message
				}
			}
		}

		return nil
	}
}
