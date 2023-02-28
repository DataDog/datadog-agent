// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// The maximum number of logs that will be buffered during the init phase before the first invocation
	maxBufferedLogs = 2000
)

// Tags contains the actual array of Tags (useful for passing it via reference)
type Tags struct {
	Tags []string
}

// LambdaLogsCollector is the route to which the AWS environment is sending the logs
// for the extension to collect them.
type LambdaLogsCollector struct {
	In                     chan []LambdaLogAPIMessage
	lastRequestID          string
	coldstartRequestID     string
	outOfMemory            bool
	out                    chan<- *logConfig.ChannelMessage
	demux                  aggregator.Demultiplexer
	extraTags              *Tags
	logsEnabled            bool
	enhancedMetricsEnabled bool
	invocationStartTime    time.Time
	invocationEndTime      time.Time
	process_once           *sync.Once
	executionContext       *executioncontext.ExecutionContext
	initDurationChan       chan<- float64

	arn string

	// handleRuntimeDone is the function to be called when a platform.runtimeDone log message is received
	handleRuntimeDone func()
}

func NewLambdaLogCollector(out chan<- *logConfig.ChannelMessage, demux aggregator.Demultiplexer, extraTags *Tags, logsEnabled bool, enhancedMetricsEnabled bool, executionContext *executioncontext.ExecutionContext, handleRuntimeDone func(), initDurationChan chan<- float64) *LambdaLogsCollector {

	return &LambdaLogsCollector{
		In:                     make(chan []LambdaLogAPIMessage, maxBufferedLogs), // Buffered, so we can hold start-up logs before first invocation without blocking
		out:                    out,
		demux:                  demux,
		extraTags:              extraTags,
		logsEnabled:            logsEnabled,
		enhancedMetricsEnabled: enhancedMetricsEnabled,
		executionContext:       executionContext,
		handleRuntimeDone:      handleRuntimeDone,
		process_once:           &sync.Once{},
		initDurationChan:       initDurationChan,
	}
}

// Start processing logs. Can be called multiple times, but only the first invocation will be effective.
func (lc *LambdaLogsCollector) Start() {
	if lc == nil {
		return
	}
	lc.process_once.Do(func() {
		// After a timeout, there may be queued logs that will be immediately sent to the logs API.
		// We want to use the restored execution context for those logs.
		state := lc.executionContext.GetCurrentState()

		log.Debugf("Starting Log Collection with ARN: %s and RequestId: %s", state.ARN, state.LastLogRequestID)

		// Once we have an ARN, we can start processing logs
		lc.arn = state.ARN
		lc.lastRequestID = state.LastRequestID
		lc.coldstartRequestID = state.ColdstartRequestID
		lc.invocationStartTime = state.StartTime
		lc.invocationEndTime = state.EndTime

		go func() {
			for messages := range lc.In {
				lc.processLogMessages(messages)
			}
			// Store the execution context if an out of memory is detected
			if lc.outOfMemory {
				err := lc.executionContext.SaveCurrentExecutionContext()
				if err != nil {
					log.Warnf("Unable to save the current state. Failed with: %s", err)
				}
			}
		}()
	})
}

// Shutdown the log collector
func (lc *LambdaLogsCollector) Shutdown() {
	close(lc.In)
}

// shouldProcessLog returns whether or not the log should be further processed.
func shouldProcessLog(message *LambdaLogAPIMessage) bool {
	if message.logType == logTypePlatformInitReport {
		return true
	}
	// Making sure that empty logs are not uselessly sent
	if len(message.stringRecord) == 0 && len(message.objectRecord.requestID) == 0 {
		return false
	}

	return true
}

func createStringRecordForReportLog(startTime, endTime time.Time, l *LambdaLogAPIMessage) string {
	runtimeDurationMs := float64(endTime.Sub(startTime).Milliseconds())
	postRuntimeDurationMs := l.objectRecord.reportLogItem.durationMs - runtimeDurationMs
	stringRecord := fmt.Sprintf("REPORT RequestId: %s\tDuration: %.2f ms\tRuntime Duration: %.2f ms\tPost Runtime Duration: %.2f ms\tBilled Duration: %d ms\tMemory Size: %d MB\tMax Memory Used: %d MB",
		l.objectRecord.requestID,
		l.objectRecord.reportLogItem.durationMs,
		runtimeDurationMs,
		postRuntimeDurationMs,
		l.objectRecord.reportLogItem.billedDurationMs,
		l.objectRecord.reportLogItem.memorySizeMB,
		l.objectRecord.reportLogItem.maxMemoryUsedMB,
	)
	initDurationMs := l.objectRecord.reportLogItem.initDurationMs
	if initDurationMs > 0 {
		stringRecord = stringRecord + fmt.Sprintf("\tInit Duration: %.2f ms", initDurationMs)
	}

	return stringRecord
}

// removeInvalidTracingItem is a temporary fix to handle malformed JSON tracing object
func removeInvalidTracingItem(data []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), ",\"tracing\":}", ""))
}

func (lc *LambdaLogsCollector) processLogMessages(messages []LambdaLogAPIMessage) {
	// sort messages by time (all from the same time zone) in ascending order.
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].time.Before(messages[j].time)
	})
	for _, message := range messages {
		lc.processMessage(&message)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if lc.logsEnabled {
			// Do not send platform log messages without a stringRecord to the intake
			if message.stringRecord == "" && message.logType != logTypeFunction {
				continue
			}
			if message.objectRecord.requestID != "" {
				lc.out <- logConfig.NewChannelMessageFromLambda([]byte(message.stringRecord), message.time, lc.arn, message.objectRecord.requestID)
			} else {
				lc.out <- logConfig.NewChannelMessageFromLambda([]byte(message.stringRecord), message.time, lc.arn, lc.lastRequestID)
			}
		}
	}
}

// processMessage performs logic about metrics and tags on the message
func (lc *LambdaLogsCollector) processMessage(
	message *LambdaLogAPIMessage,
) {
	// Do not send logs or metrics if we can't associate them with an ARN or Request ID
	if !shouldProcessLog(message) {
		return
	}
	if message.logType == logTypePlatformInitReport {
		lc.initDurationChan <- message.objectRecord.reportLogItem.initDurationTelemetry
	}

	if message.logType == logTypePlatformStart {
		if len(lc.coldstartRequestID) == 0 {
			lc.coldstartRequestID = message.objectRecord.requestID
		}
		lc.lastRequestID = message.objectRecord.requestID
		lc.invocationStartTime = message.time

		lc.executionContext.UpdateStartTime(lc.invocationStartTime)
	}

	if lc.enhancedMetricsEnabled {
		tags := tags.AddColdStartTag(lc.extraTags.Tags, lc.lastRequestID == lc.coldstartRequestID)
		if message.logType == logTypeFunction && !lc.outOfMemory {
			if lc.outOfMemory = serverlessMetrics.ContainsOutOfMemoryLog(message.stringRecord); lc.outOfMemory {
				serverlessMetrics.GenerateEnhancedMetricsFromFunctionLog(message.time, tags, lc.demux)
			}
		}
		if message.logType == logTypePlatformReport {
			args := serverlessMetrics.GenerateEnhancedMetricsFromReportLogArgs{
				InitDurationMs:   message.objectRecord.reportLogItem.initDurationMs,
				DurationMs:       message.objectRecord.reportLogItem.durationMs,
				BilledDurationMs: message.objectRecord.reportLogItem.billedDurationMs,
				MemorySizeMb:     message.objectRecord.reportLogItem.memorySizeMB,
				MaxMemoryUsedMb:  message.objectRecord.reportLogItem.maxMemoryUsedMB,
				RuntimeStart:     lc.invocationStartTime,
				RuntimeEnd:       lc.invocationEndTime,
				T:                message.time,
				Tags:             tags,
				Demux:            lc.demux,
			}
			serverlessMetrics.GenerateEnhancedMetricsFromReportLog(args)
			message.stringRecord = createStringRecordForReportLog(lc.invocationStartTime, lc.invocationEndTime, message)
		}
		if message.logType == logTypePlatformRuntimeDone {
			serverlessMetrics.GenerateEnhancedMetricsFromRuntimeDoneLog(
				serverlessMetrics.GenerateEnhancedMetricsFromRuntimeDoneLogArgs{
					Start:            lc.invocationStartTime,
					End:              message.time,
					ResponseLatency:  message.objectRecord.runtimeDoneItem.responseLatency,
					ResponseDuration: message.objectRecord.runtimeDoneItem.responseDuration,
					ProducedBytes:    message.objectRecord.runtimeDoneItem.producedBytes,
					Tags:             tags,
					Demux:            lc.demux,
				})
			lc.invocationEndTime = message.time
			lc.executionContext.UpdateEndTime(message.time)
		}
	}

	// If we receive a runtimeDone log message for the current invocation, we know the runtime is done
	// If we receive a runtimeDone message for a different invocation, we received the message too late and we ignore it
	if message.logType == logTypePlatformRuntimeDone {
		if lc.lastRequestID == message.objectRecord.requestID {
			log.Debugf("Received a runtimeDone log message for the current invocation %s", message.objectRecord.requestID)
			lc.handleRuntimeDone()
		} else {
			log.Debugf("Received a runtimeDone log message for the non-current invocation %s, ignoring it", message.objectRecord.requestID)
		}
	}
}
