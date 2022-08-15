// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tags contains the actual array of Tags (useful for passing it via reference)
type Tags struct {
	Tags []string
}

// LambdaLogsCollector is the route to which the AWS environment is sending the logs
// for the extension to collect them.
type LambdaLogsCollector struct {
	LogChannel             chan *logConfig.ChannelMessage
	Demux                  aggregator.Demultiplexer
	ExtraTags              *Tags
	LogsEnabled            bool
	EnhancedMetricsEnabled bool
	ExecutionContext       *executioncontext.ExecutionContext
	// HandleRuntimeDone is the function to be called when a platform.runtimeDone log message is received
	HandleRuntimeDone func()
}

// platformObjectRecord contains additional information found in Platform log messages
type platformObjectRecord struct {
	requestID       string           // uuid; present in LogTypePlatform{Start,End,Report}
	startLogItem    startLogItem     // present in LogTypePlatformStart only
	reportLogItem   reportLogMetrics // present in LogTypePlatformReport only
	runtimeDoneItem runtimeDoneItem  // present in LogTypePlatformRuntimeDone only
}

// reportLogMetrics contains metrics found in a LogTypePlatformReport log
type reportLogMetrics struct {
	durationMs       float64
	billedDurationMs int
	memorySizeMB     int
	maxMemoryUsedMB  int
	initDurationMs   float64
}

type runtimeDoneItem struct {
	status string
}

type startLogItem struct {
	version string
}

// logMessageTimeLayout is the layout string used to format timestamps from logs
const logMessageTimeLayout = "2006-01-02T15:04:05.999Z"

const (
	// logTypeExtension is used to represent logs messages emitted by extensions
	logTypeExtension = "extension"

	// logTypeFunction is used to represent logs messages emitted by the function
	logTypeFunction = "function"

	// logTypePlatformStart is used for the log message about the platform starting
	logTypePlatformStart = "platform.start"
	// logTypePlatformEnd is used for the log message about the platform shutting down
	logTypePlatformEnd = "platform.end"
	// logTypePlatformReport is used for the log messages containing a report of the last invocation.
	logTypePlatformReport = "platform.report"
	// logTypePlatformLogsDropped is used when AWS has dropped logs because we were unable to consume them fast enough.
	logTypePlatformLogsDropped = "platform.logsDropped"
	// logTypePlatformLogsSubscription is used for the log messages about Logs API registration
	logTypePlatformLogsSubscription = "platform.logsSubscription"
	// logTypePlatformExtension is used for the log messages about Extension API registration
	logTypePlatformExtension = "platform.extension"
	// logTypePlatformRuntimeDone is received when the runtime (customer's code) has returned (success or error)
	logTypePlatformRuntimeDone = "platform.runtimeDone"
)

// logMessage is a log message sent by the AWS API.
type logMessage struct {
	time    time.Time
	logType string
	// stringRecord is a string representation of the message's contents. It can be either received directly
	// from the logs API or added by the extension after receiving it.
	stringRecord string
	objectRecord platformObjectRecord
}

// UnmarshalJSON unmarshals the given bytes in a LogMessage object.
func (l *logMessage) UnmarshalJSON(data []byte) error {
	var j map[string]interface{}
	if err := json.Unmarshal(data, &j); err != nil {
		return fmt.Errorf("LogMessage.UnmarshalJSON: can't unmarshal json: %s", err)
	}

	var typ string
	var ok bool

	// type

	if typ, ok = j["type"].(string); !ok {
		return fmt.Errorf("LogMessage.UnmarshalJSON: malformed log message")
	}

	// time

	if timeStr, ok := j["time"].(string); ok {
		if time, err := time.Parse(logMessageTimeLayout, timeStr); err == nil {
			l.time = time
		}
	}

	// the rest

	switch typ {
	case logTypePlatformLogsSubscription, logTypePlatformExtension:
		l.logType = typ
	case logTypeFunction, logTypeExtension:
		l.logType = typ
		l.stringRecord = j["record"].(string)
	case logTypePlatformStart, logTypePlatformEnd, logTypePlatformReport, logTypePlatformRuntimeDone:
		l.logType = typ
		if objectRecord, ok := j["record"].(map[string]interface{}); ok {
			// all of these have the requestId
			if requestID, ok := objectRecord["requestId"].(string); ok {
				l.objectRecord.requestID = requestID
			}

			switch typ {
			case logTypePlatformStart:
				if version, ok := objectRecord["version"].(string); ok {
					l.objectRecord.startLogItem.version = version
				}
				l.stringRecord = fmt.Sprintf("START RequestId: %s Version: %s",
					l.objectRecord.requestID,
					l.objectRecord.startLogItem.version,
				)
			case logTypePlatformEnd:
				l.stringRecord = fmt.Sprintf("END RequestId: %s",
					l.objectRecord.requestID,
				)
			case logTypePlatformReport:
				if metrics, ok := objectRecord["metrics"].(map[string]interface{}); ok {
					if v, ok := metrics["durationMs"].(float64); ok {
						l.objectRecord.reportLogItem.durationMs = v
					}
					if v, ok := metrics["billedDurationMs"].(float64); ok {
						l.objectRecord.reportLogItem.billedDurationMs = int(v)
					}
					if v, ok := metrics["memorySizeMB"].(float64); ok {
						l.objectRecord.reportLogItem.memorySizeMB = int(v)
					}
					if v, ok := metrics["maxMemoryUsedMB"].(float64); ok {
						l.objectRecord.reportLogItem.maxMemoryUsedMB = int(v)
					}
					if v, ok := metrics["initDurationMs"].(float64); ok {
						l.objectRecord.reportLogItem.initDurationMs = v
					}
					log.Debugf("Enhanced metrics: %+v\n", l.objectRecord.reportLogItem)
				} else {
					log.Error("LogMessage.UnmarshalJSON: can't read the metrics object")
				}
			case logTypePlatformRuntimeDone:
				if status, ok := objectRecord["status"].(string); ok {
					l.objectRecord.runtimeDoneItem.status = status
				} else {
					log.Debug("Can't read the status from runtimeDone log message")
				}
			}
		} else {
			log.Error("LogMessage.UnmarshalJSON: can't read the record object")
		}
	default:
		// we're not parsing this kind of message yet
	}

	return nil
}

// shouldProcessLog returns whether or not the log should be further processed.
func shouldProcessLog(ecs *executioncontext.State, message *logMessage) bool {
	// If the global request ID or ARN variable isn't set at this point, do not process further
	if len(ecs.ARN) == 0 || len(ecs.LastRequestID) == 0 {
		return false
	}
	// Making sure that we do not process these types of logs since they are not tied to specific invovations
	if message.logType == logTypePlatformExtension || message.logType == logTypePlatformLogsSubscription {
		return false
	}
	// Making sure that empty logs are not uselessly sent
	if len(message.stringRecord) == 0 && len(message.objectRecord.requestID) == 0 {
		return false
	}

	return true
}

func createStringRecordForReportLog(l *logMessage, ecs executioncontext.State) string {
	runtimeDurationMs := float64(ecs.EndTime.Sub(ecs.StartTime).Milliseconds())
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

// parseLogsAPIPayload transforms the payload received from the Logs API to an array of LogMessage
func parseLogsAPIPayload(data []byte) ([]logMessage, error) {
	var messages []logMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		// Temporary fix to handle malformed JSON tracing object : retry with sanitization
		log.Debug("Can't read log message, retry with sanitization")
		sanitizedData := removeInvalidTracingItem(data)
		if err := json.Unmarshal(sanitizedData, &messages); err != nil {
			return nil, errors.New("can't read log message")
		}
		return messages, nil
	}
	return messages, nil
}

// removeInvalidTracingItem is a temporary fix to handle malformed JSON tracing object
func removeInvalidTracingItem(data []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), ",\"tracing\":}", ""))
}

// ServeHTTP - see type LambdaLogsCollector comment.
func (c *LambdaLogsCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	messages, err := parseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		processLogMessages(c, messages)
		w.WriteHeader(200)
	}
}

func processLogMessages(c *LambdaLogsCollector, messages []logMessage) {
	for _, message := range messages {
		processMessage(&message, c.ExecutionContext, c.EnhancedMetricsEnabled, c.ExtraTags.Tags, c.Demux, c.HandleRuntimeDone)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if c.LogsEnabled {
			// Do not send platform log messages without a stringRecord to the intake
			if message.stringRecord == "" && message.logType != logTypeFunction {
				continue
			}
			ecs := c.ExecutionContext.GetCurrentState()
			logMessage := logConfig.NewChannelMessageFromLambda([]byte(message.stringRecord), message.time, ecs.ARN, ecs.LastRequestID)
			c.LogChannel <- logMessage
		}
	}
}

// processMessage performs logic about metrics and tags on the message
func processMessage(
	message *logMessage,
	ec *executioncontext.ExecutionContext,
	enhancedMetricsEnabled bool,
	metricTags []string,
	demux aggregator.Demultiplexer,
	handleRuntimeDone func(),
) {
	ecs := ec.GetCurrentState()
	// Do not send logs or metrics if we can't associate them with an ARN or Request ID
	if !shouldProcessLog(&ecs, message) {
		return
	}

	if message.logType == logTypePlatformStart {
		lastLogRequestID := message.objectRecord.requestID
		startTime := message.time
		ec.UpdateFromStartLog(lastLogRequestID, startTime)
		ecs = ec.GetCurrentState()
	}

	if enhancedMetricsEnabled {
		tags := tags.AddColdStartTag(metricTags, ecs.LastLogRequestID == ecs.ColdstartRequestID)
		if message.logType == logTypeFunction {
			serverlessMetrics.GenerateEnhancedMetricsFromFunctionLog(message.stringRecord, message.time, tags, demux)
		}
		if message.logType == logTypePlatformReport {
			args := serverlessMetrics.GenerateEnhancedMetricsFromReportLogArgs{
				InitDurationMs:   message.objectRecord.reportLogItem.initDurationMs,
				DurationMs:       message.objectRecord.reportLogItem.durationMs,
				BilledDurationMs: message.objectRecord.reportLogItem.billedDurationMs,
				MemorySizeMb:     message.objectRecord.reportLogItem.memorySizeMB,
				MaxMemoryUsedMb:  message.objectRecord.reportLogItem.maxMemoryUsedMB,
				RuntimeStart:     ecs.StartTime,
				RuntimeEnd:       ecs.EndTime,
				T:                message.time,
				Tags:             tags,
				Demux:            demux,
			}
			serverlessMetrics.GenerateEnhancedMetricsFromReportLog(args)
			message.stringRecord = createStringRecordForReportLog(message, ecs)
		}
		if message.logType == logTypePlatformRuntimeDone {
			serverlessMetrics.GenerateRuntimeDurationMetric(ecs.StartTime, message.time, message.objectRecord.runtimeDoneItem.status, tags, demux)
			ec.UpdateFromRuntimeDoneLog(message.time)
			ecs = ec.GetCurrentState()
		}
	}

	if message.logType == logTypePlatformLogsDropped {
		log.Debug("Logs were dropped by the AWS Lambda Logs API")
	}

	// If we receive a runtimeDone log message for the current invocation, we know the runtime is done
	// If we receive a runtimeDone message for a different invocation, we received the message too late and we ignore it
	if message.logType == logTypePlatformRuntimeDone {
		if ecs.LastRequestID == message.objectRecord.requestID {
			log.Debugf("Received a runtimeDone log message for the current invocation %s", message.objectRecord.requestID)
			handleRuntimeDone()
		} else {
			log.Debugf("Received a runtimeDone log message for the non-current invocation %s, ignoring it", message.objectRecord.requestID)
		}
	}
}
