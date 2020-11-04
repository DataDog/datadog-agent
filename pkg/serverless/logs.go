package serverless

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	logTypeExtension = "extension"

	logTypeFunction = "function"

	logTypePlatformStart  = "platform.start"
	logTypePlatformEnd    = "platform.end"
	logTypePlatformReport = "platform.report"
)

// LogMessage is a log message sent by the AWS API.
type LogMessage struct {
	Time time.Time
	Type string
	// "extension" / "function" log messages contain a record which basically a log string
	StringRecord string `json:"record"`
	// platform log messages contain a struct record object
	ObjectRecord struct {
		RequestId string // uuid
		Metrics   struct {
			DurationMs       float64
			BilledDurationMs int
			MemorySizeMB     int
			MaxMemoryUsedMB  int
			InitDurationMs   float64
		}
	}
}

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
		if time, err := time.Parse("2006-01-02T15:04:05Z", timeStr); err != nil {
			// error happened, don't log anything here it could be too verbose
			// TODO(remy): at some point we will probably want to report this
		} else {
			l.Time = time
		}
	}

	// the rest

	switch typ {
	case logTypeExtension:
		fallthrough
	case logTypeFunction:
		l.Type = typ
		l.StringRecord = j["record"].(string)
	case logTypePlatformStart, logTypePlatformEnd, logTypePlatformReport:
		l.Type = typ
		if objectRecord, ok := j["record"].(map[string]interface{}); ok {
			// all of these have the requestId
			if requestId, ok := objectRecord["requestId"].(string); ok {
				l.ObjectRecord.RequestId = requestId
			}
			// only logTypePlatformReport has what we call "enhanced metrics"
			if typ == logTypePlatformReport {
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
			}
		} else {
			log.Error("LogMessage.UnmarshalJSON: can't read the record object")
		}
	default:
		// we're not parsing this kind of message yet
	}

	return nil
}
