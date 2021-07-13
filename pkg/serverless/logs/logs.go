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

	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/scheduler"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tags contains the actual array of Tags (useful for passing it via reference)
type Tags struct {
	Tags []string
}

// ExecutionContext represents the execution context
type ExecutionContext struct {
	ARN                string
	LastRequestID      string
	ColdstartRequestID string
	LastLogRequestID   string
	Coldstart          bool
}

// CollectionRouteInfo is the route on which the AWS environment is sending the logs
// for the extension to collect them. It is attached to the main HTTP server
// already receiving hits from the libraries client.
type CollectionRouteInfo struct {
	LogChannel             chan *logConfig.ChannelMessage
	MetricChannel          chan []metrics.MetricSample
	ExtraTags              *Tags
	ExecutionContext       *ExecutionContext
	LogsEnabled            bool
	EnhancedMetricsEnabled bool
}

// logMessageTimeLayout is the layout string used to format timestamps from logs
const logMessageTimeLayout = "2006-01-02T15:04:05.999Z"

const (
	// LogTypeExtension is used to represent logs messages emitted by extensions
	LogTypeExtension = "extension"

	// LogTypeFunction is used to represent logs messages emitted by the function
	LogTypeFunction = "function"

	// LogTypePlatformStart is used for the log message about the platform starting
	LogTypePlatformStart = "platform.start"
	// LogTypePlatformEnd is used for the log message about the platform shutting down
	LogTypePlatformEnd = "platform.end"
	// LogTypePlatformReport is used for the log messages containing a report of the last invocation.
	LogTypePlatformReport = "platform.report"
	// LogTypePlatformLogsDropped is used when AWS has dropped logs because we were unable to consume them fast enough.
	LogTypePlatformLogsDropped = "platform.logsDropped"
	// LogTypePlatformLogsSubscription is used for the log messages about Logs API registration
	LogTypePlatformLogsSubscription = "platform.logsSubscription"
	// LogTypePlatformExtension is used for the log messages about Extension API registration
	LogTypePlatformExtension = "platform.extension"
)

// LogMessage is a log message sent by the AWS API.
type LogMessage struct {
	Time time.Time
	ARN  string
	Type string
	// "extension" / "function" log messages contain a record which is basically a log string
	StringRecord string `json:"record"`
	ObjectRecord serverlessMetrics.PlatformObjectRecord
}

// UnmarshalJSON unmarshals the given bytes in a LogMessage object.
func (l *LogMessage) UnmarshalJSON(data []byte) error {
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
			l.Time = time
		}
	}

	// the rest

	switch typ {
	case LogTypePlatformLogsSubscription, LogTypePlatformExtension:
		l.Type = typ
	case LogTypeFunction, LogTypeExtension:
		l.Type = typ
		l.StringRecord = j["record"].(string)
	case LogTypePlatformStart, LogTypePlatformEnd, LogTypePlatformReport:
		l.Type = typ
		if objectRecord, ok := j["record"].(map[string]interface{}); ok {
			// all of these have the requestId
			if requestID, ok := objectRecord["requestId"].(string); ok {
				l.ObjectRecord.RequestID = requestID
			}

			switch typ {
			case LogTypePlatformStart:
				if version, ok := objectRecord["version"].(string); ok {
					l.ObjectRecord.Version = version
				}
				l.StringRecord = fmt.Sprintf("START RequestId: %s Version: %s",
					l.ObjectRecord.RequestID,
					l.ObjectRecord.Version,
				)
			case LogTypePlatformEnd:
				l.StringRecord = fmt.Sprintf("END RequestId: %s",
					l.ObjectRecord.RequestID,
				)
			case LogTypePlatformReport:
				if metrics, ok := objectRecord["metrics"].(map[string]interface{}); ok {
					if v, ok := metrics["durationMs"].(float64); ok {
						l.ObjectRecord.Metrics.DurationMs = v
					}
					if v, ok := metrics["billedDurationMs"].(float64); ok {
						l.ObjectRecord.Metrics.BilledDurationMs = int(v)
					}
					if v, ok := metrics["memorySizeMB"].(float64); ok {
						l.ObjectRecord.Metrics.MemorySizeMB = int(v)
					}
					if v, ok := metrics["maxMemoryUsedMB"].(float64); ok {
						l.ObjectRecord.Metrics.MaxMemoryUsedMB = int(v)
					}
					if v, ok := metrics["initDurationMs"].(float64); ok {
						l.ObjectRecord.Metrics.InitDurationMs = v
					}
					log.Debugf("Enhanced metrics: %+v\n", l.ObjectRecord.Metrics)
				} else {
					log.Error("LogMessage.UnmarshalJSON: can't read the metrics object")
				}
				l.StringRecord = createStringRecordForReportLog(l)
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
func shouldProcessLog(executionContext *ExecutionContext, message LogMessage) bool {
	// If the global request ID or ARN variable isn't set at this point, do not process further
	if len(executionContext.ARN) == 0 || len(executionContext.LastRequestID) == 0 {
		return false
	}
	// Making sure that we do not process these types of logs since they are not tied to specific invovations
	if message.Type == LogTypePlatformExtension || message.Type == LogTypePlatformLogsSubscription {
		return false
	}
	return true
}

func createStringRecordForReportLog(l *LogMessage) string {
	stringRecord := fmt.Sprintf("REPORT RequestId: %s\tDuration: %.2f ms\tBilled Duration: %d ms\tMemory Size: %d MB\tMax Memory Used: %d MB",
		l.ObjectRecord.RequestID,
		l.ObjectRecord.Metrics.DurationMs,
		l.ObjectRecord.Metrics.BilledDurationMs,
		l.ObjectRecord.Metrics.MemorySizeMB,
		l.ObjectRecord.Metrics.MaxMemoryUsedMB,
	)
	if l.ObjectRecord.Metrics.InitDurationMs > 0 {
		stringRecord = stringRecord + fmt.Sprintf("\tInit Duration: %.2f ms", l.ObjectRecord.Metrics.InitDurationMs)
	}

	return stringRecord
}

// parseLogsAPIPayload transforms the payload received from the Logs API to an array of LogMessage
func parseLogsAPIPayload(data []byte) ([]LogMessage, error) {
	var messages []LogMessage
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

// GetLambdaSource returns the LogSource used by the extension
func GetLambdaSource() *logConfig.LogSource {
	currentScheduler := scheduler.GetScheduler()
	if currentScheduler != nil {
		source := currentScheduler.GetSourceFromName("lambda")
		if source != nil {
			return source
		}
	}
	log.Debug("Impossible to retrieve the lambda LogSource")
	return nil
}

// ServeHTTP - see type CollectionRouteInfo comment.
func (c *CollectionRouteInfo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func processLogMessages(c *CollectionRouteInfo, messages []LogMessage) {
	for _, message := range messages {
		processMessage(message, c.ExecutionContext, c.EnhancedMetricsEnabled, c.ExtraTags.Tags, c.MetricChannel)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if c.LogsEnabled {
			logMessage := logConfig.NewChannelMessageFromLambda([]byte(message.StringRecord), message.Time, c.ExecutionContext.ARN, c.ExecutionContext.LastRequestID)
			c.LogChannel <- logMessage
		}
	}
}

// processMessage performs logic about metrics and tags on the message
func processMessage(message LogMessage, executionContext *ExecutionContext, enhancedMetricsEnabled bool, metricTags []string, metricsChan chan []metrics.MetricSample) {
	// Do not send logs or metrics if we can't associate them with an ARN or Request ID
	if !shouldProcessLog(executionContext, message) {
		return
	}

	if message.Type == LogTypePlatformStart {
		executionContext.LastLogRequestID = message.ObjectRecord.RequestID
	}

	if enhancedMetricsEnabled {
		tags := tags.AddColdStartTag(metricTags, executionContext.LastLogRequestID == executionContext.ColdstartRequestID)
		if message.Type == LogTypeFunction {
			serverlessMetrics.GenerateEnhancedMetricsFromFunctionLog(message.StringRecord, message.Time, tags, metricsChan)
		}
		if message.Type == LogTypePlatformReport {
			serverlessMetrics.GenerateEnhancedMetricsFromReportLog(message.ObjectRecord, message.Time, tags, metricsChan)
		}
	}

	if message.Type == LogTypePlatformLogsDropped {
		log.Debug("Logs were dropped by the AWS Lambda Logs API")
	}
}
