// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aws

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	ObjectRecord PlatformObjectRecord
}

// PlatformObjectRecord contains additional information found in Platform log messages
type PlatformObjectRecord struct {
	RequestID string           // uuid; present in LogTypePlatform{Start,End,Report}
	Version   string           // present in LogTypePlatformStart only
	Metrics   ReportLogMetrics // present in LogTypePlatformReport only
}

// ReportLogMetrics contains metrics found in a LogTypePlatformReport log
type ReportLogMetrics struct {
	DurationMs       float64
	BilledDurationMs int
	MemorySizeMB     int
	MaxMemoryUsedMB  int
	InitDurationMs   float64
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
				SetRequestID(l.ObjectRecord.RequestID)
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

// ShouldProcessLog returns whether or not the log should be further processed.
func ShouldProcessLog(arn string, lastRequestID string, message LogMessage) bool {
	// If the global request ID or ARN variable isn't set at this point, do not process further
	if arn == "" || lastRequestID == "" {
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

// ParseLogsAPIPayload transforms the payload received from the Logs API to an array of LogMessage
func ParseLogsAPIPayload(data []byte) ([]LogMessage, error) {
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
