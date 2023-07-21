package inferredspan

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

type rawTraceContext struct {
	TraceID  string `json:"x-datadog-trace-id"`
	ParentID string `json:"x-datadog-parent-id"`
}

type convertedTraceContext struct {
	TraceID  *uint64
	ParentID *uint64
}

type customMessageAttributeStruct struct {
	Type  string `json:"Type"`
	Value string `json:"Value"`
}
type bodyStruct struct {
	MessageAttributes map[string]customMessageAttributeStruct `json:"MessageAttributes"`
}

func extractTraceContextFromSNSSQSEvent(firstRecord events.SQSMessage) *rawTraceContext {
	var messageBody bodyStruct
	err := json.Unmarshal([]byte(firstRecord.Body), &messageBody)
	if err != nil {
		log.Debug("Error unmarshaling the message body: ", err)
		return nil
	}

	ddCustomPayloadValue, ok := messageBody.MessageAttributes["_datadog"]
	if !ok {
		log.Debug("No Datadog trace context found")
		return nil
	}

	var traceData rawTraceContext
	if ddCustomPayloadValue.Type == "Binary" {
		decodedBinary, err := base64.StdEncoding.DecodeString(string(ddCustomPayloadValue.Value))
		if err != nil {
			log.Debug("Error decoding binary: ", err)
			return nil
		}
		err = json.Unmarshal(decodedBinary, &traceData)
		if err != nil {
			log.Debug("Error unmarshaling the decoded binary: ", err)
			return nil
		}
	} else {
		log.Debug("Unsupported DataType in _datadog payload")
		return nil
	}

	return &traceData
}

func extractTraceContextFromPureSqsEvent(ddPayloadValue events.SQSMessageAttribute) *rawTraceContext {
	var traceData rawTraceContext
	if ddPayloadValue.DataType == "String" {
		err := json.Unmarshal([]byte(*ddPayloadValue.StringValue), &traceData)
		if err != nil {
			log.Debug("Error unmarshaling payload value: ", err)
			return nil
		}
	} else {
		log.Debug("Unsupported DataType in _datadog payload")
		return nil
	}

	return &traceData
}

func convertRawTraceContext(rawTrace *rawTraceContext) *convertedTraceContext {
	var uint64TraceID, uint64ParentID *uint64

	if rawTrace.TraceID != "" {
		parsedTraceID, err := strconv.ParseUint(rawTrace.TraceID, 10, 64)
		if err != nil {
			log.Debug("Error parsing trace ID: ", err)
			return nil
		}
		uint64TraceID = &parsedTraceID
	}

	if rawTrace.ParentID != "" {
		parsedParentID, err := strconv.ParseUint(rawTrace.ParentID, 10, 64)
		if err != nil {
			log.Debug("Error parsing parent ID: ", err)
			return nil
		}
		uint64ParentID = &parsedParentID
	}

	if uint64TraceID == nil || uint64ParentID == nil {
		return nil
	}

	return &convertedTraceContext{
		TraceID:  uint64TraceID,
		ParentID: uint64ParentID,
	}
}
