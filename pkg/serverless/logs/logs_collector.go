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

	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	serverlessStreamLogs "github.com/DataDog/datadog-agent/pkg/serverless/streamlogs"
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

//nolint:revive // TODO(SERV) Fix revive linter
type LambdaInitMetric struct {
	InitDurationTelemetry float64
	InitStartTime         time.Time
}

// LambdaLogsCollector is the route to which the AWS environment is sending the logs
// for the extension to collect them.
type LambdaLogsCollector struct {
	In                     chan []LambdaLogAPIMessage
	lastRequestID          string
	coldstartRequestID     string
	lastOOMRequestID       string
	out                    chan<- *logConfig.ChannelMessage
	demux                  aggregator.Demultiplexer
	extraTags              *Tags
	logsEnabled            bool
	enhancedMetricsEnabled bool
	invocationStartTime    time.Time
	invocationEndTime      time.Time
	//nolint:revive // TODO(SERV) Fix revive linter
	process_once         *sync.Once
	executionContext     *executioncontext.ExecutionContext
	lambdaInitMetricChan chan<- *LambdaInitMetric
	orphanLogsChan       chan []LambdaLogAPIMessage

	arn string

	// handleRuntimeDone is the function to be called when a platform.runtimeDone log message is received
	handleRuntimeDone func()
}

//nolint:revive // TODO(SERV) Fix revive linter
func NewLambdaLogCollector(out chan<- *logConfig.ChannelMessage, demux aggregator.Demultiplexer, extraTags *Tags, logsEnabled bool, enhancedMetricsEnabled bool, executionContext *executioncontext.ExecutionContext, handleRuntimeDone func(), lambdaInitMetricChan chan<- *LambdaInitMetric) *LambdaLogsCollector {

	return &LambdaLogsCollector{
		In:                     make(chan []LambdaLogAPIMessage),
		out:                    out,
		demux:                  demux,
		extraTags:              extraTags,
		logsEnabled:            logsEnabled,
		enhancedMetricsEnabled: enhancedMetricsEnabled,
		executionContext:       executionContext,
		handleRuntimeDone:      handleRuntimeDone,
		process_once:           &sync.Once{},
		lambdaInitMetricChan:   lambdaInitMetricChan,
		orphanLogsChan:         make(chan []LambdaLogAPIMessage, maxBufferedLogs),
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
		lc.lastOOMRequestID = state.LastOOMRequestID
		lc.invocationStartTime = state.StartTime
		lc.invocationEndTime = state.EndTime

		go func() {
			for messages := range lc.In {
				lc.processLogMessages(messages)
			}

			// Process logs without a request ID when it becomes available
			if len(lc.lastRequestID) > 0 && len(lc.orphanLogsChan) > 0 {
				for msgs := range lc.orphanLogsChan {
					lc.processLogMessages(msgs)
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
	if message.logType == logTypePlatformInitReport || message.logType == logTypePlatformInitStart {
		return true
	}
	// Making sure that empty logs are not uselessly sent
	if len(message.stringRecord) == 0 && len(message.objectRecord.requestID) == 0 {
		return false
	}

	return true
}

// calculateRuntimeDuration returns the runtimeDuration and postRuntimeDuration is milliseconds
func calculateRuntimeDuration(l *LambdaLogAPIMessage, startTime, endTime time.Time) (float64, float64) {
	// If neither startTime nor endTime have been set, avoid returning exaggerated values
	if startTime.IsZero() || endTime.IsZero() {
		return 0, 0
	}
	runtimeDurationMs := float64(endTime.Sub(startTime).Milliseconds())
	postRuntimeDurationMs := l.objectRecord.reportLogItem.durationMs - runtimeDurationMs
	return runtimeDurationMs, postRuntimeDurationMs
}

func createStringRecordForReportLog(startTime, endTime time.Time, l *LambdaLogAPIMessage) string {
	runtimeDurationMs, postRuntimeDurationMs := calculateRuntimeDuration(l, startTime, endTime)
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

// createStringRecordForTimeoutLog returns the `Task timed out` log using the platform.report message
func createStringRecordForTimeoutLog(l *LambdaLogAPIMessage) string {
	durationMs := l.objectRecord.reportLogItem.durationMs
	durationSeconds := durationMs / 1000
	timeStr := l.time.Format(time.RFC3339Nano)
	return fmt.Sprintf(`%s %s Task timed out after %.2f seconds`, timeStr, l.objectRecord.requestID, durationSeconds)

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
	orphanMessages := []LambdaLogAPIMessage{}
	for _, message := range messages {
		lc.processMessage(&message)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if lc.logsEnabled {
			// Do not send platform log messages without a stringRecord to the intake
			if message.stringRecord == "" && message.logType != logTypeFunction {
				continue
			}

			// If logs cannot be assigned a request ID, delay sending until a request ID is available
			if len(message.objectRecord.requestID) == 0 && len(lc.lastRequestID) == 0 {
				orphanMessages = append(orphanMessages, message)
				continue
			}

			// Don't collect stream-logs output
			if serverlessStreamLogs.Is(message.stringRecord) {
				continue
			}

			isErrorLog := message.logType == logTypeFunction && serverlessMetrics.ContainsOutOfMemoryLog(message.stringRecord)
			if message.objectRecord.requestID != "" {
				lc.out <- logConfig.NewChannelMessageFromLambda([]byte(message.stringRecord), message.time, lc.arn, message.objectRecord.requestID, isErrorLog)
			} else {
				lc.out <- logConfig.NewChannelMessageFromLambda([]byte(message.stringRecord), message.time, lc.arn, lc.lastRequestID, isErrorLog)
			}

			// Create the timeout log from the REPORT log if a timeout status is detected
			isTimeoutLog := message.logType == logTypePlatformReport && message.objectRecord.status == timeoutStatus
			if isTimeoutLog {
				lc.out <- logConfig.NewChannelMessageFromLambda([]byte(createStringRecordForTimeoutLog(&message)), message.time, lc.arn, message.objectRecord.requestID, isTimeoutLog)
			}
		}
	}
	if len(orphanMessages) > 0 {
		lc.orphanLogsChan <- orphanMessages
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
		lambdaMetric := &LambdaInitMetric{
			InitDurationTelemetry: message.objectRecord.reportLogItem.initDurationTelemetry,
		}
		lc.lambdaInitMetricChan <- lambdaMetric
	}

	if message.logType == logTypePlatformInitStart {
		lambdaMetric := &LambdaInitMetric{
			InitStartTime: message.objectRecord.reportLogItem.initStartTime,
		}
		lc.lambdaInitMetricChan <- lambdaMetric
	}

	if message.logType == logTypePlatformStart {
		if len(lc.coldstartRequestID) == 0 {
			lc.coldstartRequestID = message.objectRecord.requestID
		}
		lc.lastRequestID = message.objectRecord.requestID
		lc.invocationStartTime = message.time

		lc.executionContext.UpdateStartTime(lc.invocationStartTime)
	}

	if message.logType == logTypePlatformReport {
		message.stringRecord = createStringRecordForReportLog(lc.invocationStartTime, lc.invocationEndTime, message)
	}

	if lc.enhancedMetricsEnabled {
		proactiveInit := false
		coldStart := false
		// Only run this block if the LC thinks we're in a cold start
		if lc.lastRequestID == lc.coldstartRequestID {
			coldStartTags := lc.executionContext.GetColdStartTagsForRequestID(lc.lastRequestID)
			proactiveInit = coldStartTags.IsProactiveInit
			coldStart = coldStartTags.IsColdStart
		}
		tags := tags.AddColdStartTag(lc.extraTags.Tags, coldStart, proactiveInit)
		//nolint:revive // TODO(SERV) Fix revive linter
		outOfMemoryRequestId := ""

		if message.logType == logTypeFunction {
			if lc.lastOOMRequestID != lc.lastRequestID && serverlessMetrics.ContainsOutOfMemoryLog(message.stringRecord) {
				outOfMemoryRequestId = lc.lastRequestID
			}
		}
		if message.logType == logTypePlatformReport {
			memorySize := message.objectRecord.reportLogItem.memorySizeMB
			memoryUsed := message.objectRecord.reportLogItem.maxMemoryUsedMB
			status := message.objectRecord.status
			reportOutOfMemory := memoryUsed > 0 && memoryUsed >= memorySize

			args := serverlessMetrics.GenerateEnhancedMetricsFromReportLogArgs{
				InitDurationMs:   message.objectRecord.reportLogItem.initDurationMs,
				DurationMs:       message.objectRecord.reportLogItem.durationMs,
				BilledDurationMs: message.objectRecord.reportLogItem.billedDurationMs,
				MemorySizeMb:     memorySize,
				MaxMemoryUsedMb:  memoryUsed,
				RuntimeStart:     lc.invocationStartTime,
				RuntimeEnd:       lc.invocationEndTime,
				T:                message.time,
				Tags:             tags,
				Demux:            lc.demux,
			}

			if status == errorStatus && lc.lastOOMRequestID != message.objectRecord.requestID && reportOutOfMemory {
				outOfMemoryRequestId = message.objectRecord.requestID
			}
			serverlessMetrics.GenerateEnhancedMetricsFromReportLog(args)
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
		if outOfMemoryRequestId != "" {
			lc.lastOOMRequestID = outOfMemoryRequestId
			lc.executionContext.UpdateOutOfMemoryRequestID(lc.lastOOMRequestID)
			serverlessMetrics.GenerateOutOfMemoryEnhancedMetrics(message.time, tags, lc.demux)
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
