// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package propagation manages propagation of trace context headers.
package propagation

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	json "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	awsTraceHeader   = "AWSTraceHeader"
	datadogSQSHeader = "_datadog"

	rootPrefix     = "Root="
	parentPrefix   = "Parent="
	sampledPrefix  = "Sampled="
	rootPadding    = len(rootPrefix + "1-00000000-00000000")
	parentPadding  = len(parentPrefix)
	sampledPadding = len(sampledPrefix)
)

var rootRegex = regexp.MustCompile("Root=1-[0-9a-fA-F]{8}-00000000[0-9a-fA-F]{16}")

var (
	errorAWSTraceHeaderMismatch = errors.New("AWSTraceHeader does not match expected regex")
	errorAWSTraceHeaderEmpty    = errors.New("AWSTraceHeader does not contain trace ID and parent ID")
	errorStringNotFound         = errors.New("String value not found in _datadog payload")
	errorUnsupportedDataType    = errors.New("Unsupported DataType in _datadog payload")
	errorNoDDContextFound       = errors.New("No Datadog trace context found")
	errorUnsupportedPayloadType = errors.New("Unsupported type for _datadog payload")
	errorUnsupportedTypeType    = errors.New("Unsupported type in _datadog payload")
	errorUnsupportedValueType   = errors.New("Unsupported value type in _datadog payload")
	errorUnsupportedTypeValue   = errors.New("Unsupported Type in _datadog payload")
	errorCouldNotUnmarshal      = errors.New("Could not unmarshal the invocation event payload")
)

// extractTraceContextfromAWSTraceHeader extracts trace context from the
// AWSTraceHeader directly. Unlike the other carriers in this file, it should
// not be passed to the tracer.Propagator, instead extracting context directly.
func extractTraceContextfromAWSTraceHeader(value string) (*TraceContext, error) {
	if !rootRegex.MatchString(value) {
		return nil, errorAWSTraceHeaderMismatch
	}
	var (
		startPart int
		traceID   string
		parentID  string
		sampled   string
		err       error
	)
	length := len(value)
	for startPart < length {
		endPart := strings.IndexRune(value[startPart:], ';') + startPart
		if endPart < startPart {
			endPart = length
		}
		part := value[startPart:endPart]
		if strings.HasPrefix(part, rootPrefix) {
			if traceID == "" {
				traceID = part[rootPadding:]
			}
		} else if strings.HasPrefix(part, parentPrefix) {
			if parentID == "" {
				parentID = part[parentPadding:]
			}
		} else if strings.HasPrefix(part, sampledPrefix) {
			if sampled == "" {
				sampled = part[sampledPadding:]
			}
		}
		if traceID != "" && parentID != "" && sampled != "" {
			break
		}
		startPart = endPart + 1
	}
	tc := new(TraceContext)
	tc.TraceID, err = strconv.ParseUint(traceID, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse trace ID from AWSTraceHeader: %w", err)
	}
	tc.ParentID, err = strconv.ParseUint(parentID, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse parent ID from AWSTraceHeader: %w", err)
	}
	if sampled == "1" {
		tc.SamplingPriority = sampler.PriorityAutoKeep
	}
	if tc.TraceID == 0 || tc.ParentID == 0 {
		return nil, errorAWSTraceHeaderEmpty
	}
	return tc, nil
}

// sqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessage type.
func sqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	if attr, ok := event.MessageAttributes[datadogSQSHeader]; ok {
		return sqsMessageAttrCarrier(attr)
	}
	return snsSqsMessageCarrier(event)
}

// sqsMessageAttrCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessageAttribute field on an events.SQSMessage
// type.
func sqsMessageAttrCarrier(attr events.SQSMessageAttribute) (tracer.TextMapReader, error) {
	var bytes []byte
	switch attr.DataType {
	case "String":
		if attr.StringValue == nil {
			return nil, errorStringNotFound
		}
		bytes = []byte(*attr.StringValue)
	case "Binary":
		// SNS => SQS => Lambda with SQS's subscription to SNS has enabled RAW
		// MESSAGE DELIVERY option
		bytes = attr.BinaryValue // No need to decode base64 because already decoded
	default:
		return nil, errorUnsupportedDataType
	}

	var carrier tracer.TextMapCarrier
	if err := json.Unmarshal(bytes, &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling payload value: %w", err)
	}
	return carrier, nil
}

// snsBody is used to  unmarshal only required fields on events.SNSEntity
// types.
type snsBody struct {
	MessageAttributes map[string]interface{}
}

// snsSqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the body of an events.SQSMessage type.
func snsSqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	var body snsBody
	err := json.Unmarshal([]byte(event.Body), &body)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling message body: %w", err)
	}
	return snsEntityCarrier(events.SNSEntity{
		MessageAttributes: body.MessageAttributes,
	})
}

// snsEntityCarrier returns the tracer.TextMapReader used to extract trace
// context from the attributes of an events.SNSEntity type.
func snsEntityCarrier(event events.SNSEntity) (tracer.TextMapReader, error) {
	msgAttrs, ok := event.MessageAttributes[datadogSQSHeader]
	if !ok {
		return nil, errorNoDDContextFound
	}
	mapAttrs, ok := msgAttrs.(map[string]interface{})
	if !ok {
		return nil, errorUnsupportedPayloadType
	}

	typ, ok := mapAttrs["Type"].(string)
	if !ok {
		return nil, errorUnsupportedTypeType
	}
	val, ok := mapAttrs["Value"].(string)
	if !ok {
		return nil, errorUnsupportedValueType
	}

	var bytes []byte
	var err error
	switch typ {
	case "Binary":
		bytes, err = base64.StdEncoding.DecodeString(val)
		if err != nil {
			return nil, fmt.Errorf("Error decoding binary: %w", err)
		}
	case "String":
		bytes = []byte(val)
	default:
		return nil, errorUnsupportedTypeValue
	}

	var carrier tracer.TextMapCarrier
	if err = json.Unmarshal(bytes, &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling the decoded binary: %w", err)
	}
	return carrier, nil
}

type invocationPayload struct {
	Headers tracer.TextMapCarrier `json:"headers"`
}

// rawPayloadCarrier returns the tracer.TextMapReader used to extract trace
// context from the raw json event payload.
func rawPayloadCarrier(rawPayload []byte) (tracer.TextMapReader, error) {
	var payload invocationPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, errorCouldNotUnmarshal
	}
	return payload.Headers, nil
}

// headersCarrier returns the tracer.TextMapReader used to extract trace
// context from a Headers field of form map[string]string.
func headersCarrier(hdrs map[string]string) (tracer.TextMapReader, error) {
	return tracer.TextMapCarrier(hdrs), nil
}
