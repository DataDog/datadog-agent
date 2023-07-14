// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package logsagentexporter

import (
	"context"
	"errors"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"go.uber.org/zap"
	"strings"
	"time"

	logsmapping "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs"
	"go.opentelemetry.io/collector/pdata/plog"
)

// consumeLogs is implementation of consumer.ConsumeLogsFunc
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

					content, err := ddLog.MarshalJSON()
					if err != nil {
						return err
					}

					tags := ddLog.GetDdtags()
					var service string
					if ddLog.Service != nil {
						service = *ddLog.Service
					} else {
						service = ""
					}
					status := ddLog.AdditionalProperties["status"]
					if status == "" {
						status = message.StatusInfo
					}
					timestamp, err := time.Parse(time.RFC3339, ddLog.AdditionalProperties["@timestamp"])
					if err != nil {
						return err
					}
					origin := message.NewOrigin(logSource)
					origin.SetTags(strings.Split(tags, ","))
					origin.SetService(service)

					message := message.NewMessage(content, origin, status, timestamp.Unix())

					logsAgentChannel <- message
				}
			}
		}

		return nil
	}
}
