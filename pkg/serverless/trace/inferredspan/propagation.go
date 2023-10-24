// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

const (
	awsTraceHeader = "AWSTraceHeader"
	datadogHeader  = "_datadog"

	rootPrefix    = "Root="
	parentPrefix  = "Parent="
	rootPadding   = len(rootPrefix + "1-00000000-00000000")
	parentPadding = len(parentPrefix)
)

var rootRegex = regexp.MustCompile("Root=1-[0-9a-fA-F]{8}-00000000[0-9a-fA-F]{16}")

type rawTraceContext struct {
	TraceID  string `json:"x-datadog-trace-id"`
	ParentID string `json:"x-datadog-parent-id"`
	base     int
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

func extractTraceContext(event events.SQSMessage) *convertedTraceContext {
	var rawTrace *rawTraceContext

	if awsAttribute, ok := event.Attributes[awsTraceHeader]; ok {
		rawTrace = extractTraceContextfromAWSTraceHeader(awsAttribute)
	}

	if rawTrace == nil {
		if ddMessageAttribute, ok := event.MessageAttributes[datadogHeader]; ok {
			rawTrace = extractTraceContextFromPureSqsEvent(ddMessageAttribute)
		} else {
			rawTrace = extractTraceContextFromSNSSQSEvent(event)
		}
	}

	return convertRawTraceContext(rawTrace)
}

func extractTraceContextFromSNSSQSEvent(firstRecord events.SQSMessage) *rawTraceContext {
	var messageBody bodyStruct
	err := json.Unmarshal([]byte(firstRecord.Body), &messageBody)
	if err != nil {
		log.Debug("Error unmarshaling the message body: ", err)
		return nil
	}

	ddCustomPayloadValue, ok := messageBody.MessageAttributes[datadogHeader]
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
		return &traceData
	}

	if ddPayloadValue.DataType == "Binary" {
		err := json.Unmarshal(ddPayloadValue.BinaryValue, &traceData) // No need to decode base64 because already decoded
		if err != nil {
			log.Debug("Error unmarshaling the decoded binary: ", err)
			return nil
		}
		return &traceData
	}

	log.Debug("Unsupported DataType in _datadog payload")
	return nil
}

func extractTraceContextfromAWSTraceHeader(value string) *rawTraceContext {
	if !rootRegex.MatchString(value) {
		return nil
	}

	var startPart int
	traceData := &rawTraceContext{base: 16}
	length := len(value)
	for startPart < length {
		endPart := strings.IndexRune(value[startPart:], ';') + startPart
		if endPart < startPart {
			endPart = length
		}
		part := value[startPart:endPart]
		if strings.HasPrefix(part, rootPrefix) {
			if traceData.TraceID == "" {
				traceData.TraceID = part[rootPadding:]
			}
		} else if strings.HasPrefix(part, parentPrefix) {
			if traceData.ParentID == "" {
				traceData.ParentID = part[parentPadding:]
			}
		}
		if traceData.TraceID != "" && traceData.ParentID != "" {
			break
		}
		startPart = endPart + 1
	}

	return traceData
}

func convertRawTraceContext(rawTrace *rawTraceContext) *convertedTraceContext {
	if rawTrace == nil {
		return nil
	}

	var uint64TraceID, uint64ParentID *uint64

	base := rawTrace.base
	if base == 0 {
		base = 10
	}

	if rawTrace.TraceID != "" {
		parsedTraceID, err := strconv.ParseUint(rawTrace.TraceID, base, 64)
		if err != nil {
			log.Debug("Error parsing trace ID: ", err)
			return nil
		}
		uint64TraceID = &parsedTraceID
	}

	if rawTrace.ParentID != "" {
		parsedParentID, err := strconv.ParseUint(rawTrace.ParentID, base, 64)
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
