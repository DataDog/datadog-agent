// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"fmt"
	"time"

	json "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// platformObjectRecord contains additional information found in Platform log messages
type platformObjectRecord struct {
	requestID       string           // uuid; present in LogTypePlatform{Start,End,Report}
	startLogItem    startLogItem     // present in LogTypePlatformStart only
	runtimeDoneItem runtimeDoneItem  // present in LogTypePlatformRuntimeDone only
	reportLogItem   reportLogMetrics // present in LogTypePlatformReport only
	status          string           // status is the status of either an init or invocation phase
}

// reportLogMetrics contains metrics found in a LogTypePlatformReport log
// initDurationMs is sent at the very end of the tx
// but initDurationTelemetry is also the duration in ms
// and along with initStartTime, are provided by TelemetryAPI
// early in the invocation, which is a bit confusing
// TODO Astuyve - refactor out initDurationMs to draw from TelemetryAPI
type reportLogMetrics struct {
	durationMs            float64
	billedDurationMs      int
	memorySizeMB          int
	maxMemoryUsedMB       int
	initDurationMs        float64
	initDurationTelemetry float64
	initStartTime         time.Time
}

// runtimeDoneItem contains metrics found in a LogTypePlatformRuntimeDone log
type runtimeDoneItem struct {
	responseLatency  float64
	responseDuration float64
	producedBytes    float64
}

type startLogItem struct {
	version string
}

// LambdaLogAPIMessage is a log message sent by the AWS API.
type LambdaLogAPIMessage struct {
	time    time.Time
	logType string
	// stringRecord is a string representation of the message's contents. It can be either received directly
	// from the logs API or added by the extension after receiving it.
	stringRecord string
	objectRecord platformObjectRecord
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
	// logTypePlatformReport is used for the log messages containing a report of the last invocation.
	logTypePlatformReport = "platform.report"
	// logTypePlatformLogsDropped is used when AWS has dropped logs because we were unable to consume them fast enough.
	logTypePlatformLogsDropped = "platform.logsDropped"
	// logTypePlatformRuntimeDone is received when the runtime (customer's code) has returned (success or error)
	logTypePlatformRuntimeDone = "platform.runtimeDone"
	// logTypePlatformInitReport is received when init finishes
	logTypePlatformInitReport = "platform.initReport"
	// logTypePlatformInitStart is received when init starts
	logTypePlatformInitStart = "platform.initStart"

	// errorStatus indicates the init or invoke phase has errored out
	errorStatus string = "error"
	// timeoutStatus indicates the init or invoke phase has timed out
	timeoutStatus string = "timeout"
)

// UnmarshalJSON unmarshals the given bytes in a LogMessage object.
func (l *LambdaLogAPIMessage) UnmarshalJSON(data []byte) error {
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
	case logTypePlatformLogsDropped:
		l.handleDroppedRecord(j)
	case logTypeFunction, logTypeExtension:
		l.handleFunctionAndExtensionRecord(j, typ)
	case logTypePlatformStart, logTypePlatformReport, logTypePlatformRuntimeDone, logTypePlatformInitReport, logTypePlatformInitStart:
		l.handlePlatformRecord(j, typ)
	default:
		// we're not parsing this kind of message yet
		// platform.extension, platform.logsSubscription, platform.fault
	}

	return nil
}

func (l *LambdaLogAPIMessage) handleDroppedRecord(data map[string]interface{}) {
	var reason string
	if record, ok := data["record"].(map[string]interface{}); ok {
		reason = record["reason"].(string)
	}
	log.Debugf("Logs were dropped by the AWS Lambda Logs API: %s", reason)
}

func (l *LambdaLogAPIMessage) handleFunctionAndExtensionRecord(data map[string]interface{}, typ string) {
	l.logType = typ
	l.stringRecord = data["record"].(string)
}

func (l *LambdaLogAPIMessage) handlePlatformRecord(data map[string]interface{}, typ string) {
	l.logType = typ
	objectRecord, ok := data["record"].(map[string]interface{})
	if !ok {
		log.Error("LogMessage.UnmarshalJSON: can't read the record object")
		return
	}
	// all of these have the requestId
	if requestID, ok := objectRecord["requestId"].(string); ok {
		l.objectRecord.requestID = requestID
	}

	switch typ {
	case logTypePlatformStart:
		l.handlePlatformStart(objectRecord)
	case logTypePlatformReport:
		l.handlePlatformReport(objectRecord)
	case logTypePlatformRuntimeDone:
		l.handlePlatformRuntimeDone(objectRecord)
	case logTypePlatformInitReport:
		l.handlePlatformInitReport(objectRecord)
	case logTypePlatformInitStart:
		l.handlePlatformInitStart(objectRecord)
	}
}

func (l *LambdaLogAPIMessage) handlePlatformStart(objectRecord map[string]interface{}) {
	if version, ok := objectRecord["version"].(string); ok {
		l.objectRecord.startLogItem.version = version
	}
	l.stringRecord = fmt.Sprintf("START RequestId: %s Version: %s",
		l.objectRecord.requestID,
		l.objectRecord.startLogItem.version,
	)
}

func (l *LambdaLogAPIMessage) handlePlatformReport(objectRecord map[string]interface{}) {
	if status, ok := objectRecord["status"].(string); ok {
		l.objectRecord.status = status
	}
	metrics, ok := objectRecord["metrics"].(map[string]interface{})
	if !ok {
		log.Errorf("LogMessage.UnmarshalJSON: can't read the metrics object %v", objectRecord)
		return
	}
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
}

//nolint:revive // TODO(SERV) Fix revive linter
func (l *LambdaLogAPIMessage) handlePlatformInitStart(objectRecord map[string]interface{}) {
	l.objectRecord.reportLogItem.initStartTime = l.time
}

func (l *LambdaLogAPIMessage) handlePlatformRuntimeDone(objectRecord map[string]interface{}) {
	l.stringRecord = fmt.Sprintf("END RequestId: %s", l.objectRecord.requestID)
	l.handlePlatformRuntimeDoneSpans(objectRecord)
	l.handlePlatformRuntimeDoneMetrics(objectRecord)
}

func (l *LambdaLogAPIMessage) handlePlatformRuntimeDoneSpans(objectRecord map[string]interface{}) {
	spans, ok := objectRecord["spans"].([]interface{})
	if !ok {
		// no spans if the function errored and did not return a response
		log.Debug("LogMessage.UnmarshalJSON: no spans object received")
		return
	}
	for _, span := range spans {
		spanMap, ok := span.(map[string]interface{})
		if !ok {
			continue
		}
		durationMs, ok := spanMap["durationMs"].(float64)
		if !ok {
			continue
		}
		if v, ok := spanMap["name"].(string); ok {
			switch v {
			case "responseLatency":
				l.objectRecord.runtimeDoneItem.responseLatency = durationMs
			case "responseDuration":
				l.objectRecord.runtimeDoneItem.responseDuration = durationMs
			}
		}
	}
}

func (l *LambdaLogAPIMessage) handlePlatformRuntimeDoneMetrics(objectRecord map[string]interface{}) {
	metrics, ok := objectRecord["metrics"].(map[string]interface{})
	if !ok {
		log.Error("LogMessage.UnmarshalJSON: can't read the metrics object")
		return
	}
	if v, ok := metrics["producedBytes"].(float64); ok {
		l.objectRecord.runtimeDoneItem.producedBytes = v
	}
	log.Debugf("Runtime done metrics: %+v\n", l.objectRecord.runtimeDoneItem)
}

func (l *LambdaLogAPIMessage) handlePlatformInitReport(objectRecord map[string]interface{}) {
	metrics, ok := objectRecord["metrics"].(map[string]interface{})
	if !ok {
		log.Error("LogMessage.UnmarshalJSON: can't read the metrics object")
		return
	}
	if v, ok := metrics["durationMs"].(float64); ok {
		l.objectRecord.reportLogItem.initDurationTelemetry = v
	}
	log.Debugf("InitReport done metrics: %+v\n", l.objectRecord.reportLogItem)
}

// parseLogsAPIPayload transforms the payload received from the Logs API to an array of LogMessage
func parseLogsAPIPayload(data []byte) ([]LambdaLogAPIMessage, error) {
	var messages []LambdaLogAPIMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		// Temporary fix to handle malformed JSON tracing object : retry with sanitization
		log.Debugf("Can't read log message, retry with sanitization: %s", err)
		sanitizedData := removeInvalidTracingItem(data)
		if err := json.Unmarshal(sanitizedData, &messages); err != nil {
			return nil, fmt.Errorf("can't read log message: %s", err)
		}
		return messages, nil
	}
	return messages, nil
}
