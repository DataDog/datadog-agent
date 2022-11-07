package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// platformObjectRecord contains additional information found in Platform log messages
type platformObjectRecord struct {
	requestID     string           // uuid; present in LogTypePlatform{Start,End,Report}
	startLogItem  startLogItem     // present in LogTypePlatformStart only
	reportLogItem reportLogMetrics // present in LogTypePlatformReport only
}

// reportLogMetrics contains metrics found in a LogTypePlatformReport log
type reportLogMetrics struct {
	durationMs       float64
	billedDurationMs int
	memorySizeMB     int
	maxMemoryUsedMB  int
	initDurationMs   float64
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
	// logTypePlatformEnd is used for the log message about the platform shutting down
	logTypePlatformEnd = "platform.end"
	// logTypePlatformReport is used for the log messages containing a report of the last invocation.
	logTypePlatformReport = "platform.report"
	// logTypePlatformLogsDropped is used when AWS has dropped logs because we were unable to consume them fast enough.
	logTypePlatformLogsDropped = "platform.logsDropped"
	// logTypePlatformRuntimeDone is received when the runtime (customer's code) has returned (success or error)
	logTypePlatformRuntimeDone = "platform.runtimeDone"
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
	case logTypePlatformStart, logTypePlatformEnd, logTypePlatformReport, logTypePlatformRuntimeDone:
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
	if objectRecord, ok := data["record"].(map[string]interface{}); ok {
		// all of these have the requestId
		if requestID, ok := objectRecord["requestId"].(string); ok {
			l.objectRecord.requestID = requestID
		}

		switch typ {
		case logTypePlatformStart:
			l.handlePlatormStart(objectRecord)
		case logTypePlatformEnd:
			l.handlePlatormEnd(objectRecord)
		case logTypePlatformReport:
			l.handlePlatormReport(objectRecord)
		}
	} else {
		log.Error("LogMessage.UnmarshalJSON: can't read the record object")
	}
}

func (l *LambdaLogAPIMessage) handlePlatormStart(objectRecord map[string]interface{}) {
	if version, ok := objectRecord["version"].(string); ok {
		l.objectRecord.startLogItem.version = version
	}
	l.stringRecord = fmt.Sprintf("START RequestId: %s Version: %s",
		l.objectRecord.requestID,
		l.objectRecord.startLogItem.version,
	)
}

func (l *LambdaLogAPIMessage) handlePlatormEnd(objectRecord map[string]interface{}) {
	l.stringRecord = fmt.Sprintf("END RequestId: %s",
		l.objectRecord.requestID,
	)
}

func (l *LambdaLogAPIMessage) handlePlatormReport(objectRecord map[string]interface{}) {
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
}

// parseLogsAPIPayload transforms the payload received from the Logs API to an array of LogMessage
func parseLogsAPIPayload(data []byte) ([]LambdaLogAPIMessage, error) {
	var messages []LambdaLogAPIMessage
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
