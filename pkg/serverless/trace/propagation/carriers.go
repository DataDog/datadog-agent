// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package propagation manages propagation of trace context headers.
package propagation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/aws/aws-lambda-go/events"
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

// extractTraceContextfromAWSTraceHeader extracts trace context from the
// AWSTraceHeader directly. Unlike the other carriers in this file, it should
// not be passed to the tracer.Propagator, instead extracting context directly.
func extractTraceContextfromAWSTraceHeader(value string) (*TraceContext, error) {
	if !rootRegex.MatchString(value) {
		return nil, errors.New("AWSTraceHeader does not match expected regex")
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
		return nil, errors.New("AWSTraceHeader does not contain trace ID and parent ID")
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
			return nil, errors.New("String value not found in _datadog payload")
		}
		bytes = []byte(*attr.StringValue)
	case "Binary":
		// SNS => SQS => Lambda with SQS's subscription to SNS has enabled RAW
		// MESSAGE DELIVERY option
		bytes = attr.BinaryValue // No need to decode base64 because already decoded
	default:
		return nil, errors.New("Unsupported DataType in _datadog payload")
	}

	var carrier tracer.TextMapCarrier
	if err := json.Unmarshal(bytes, &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling payload value: %w", err)
	}
	return carrier, nil
}

// snsSqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the body of an events.SQSMessage type.
func snsSqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	var body struct {
		MessageAttributes map[string]struct {
			Type  string
			Value string
		}
	}
	err := json.Unmarshal([]byte(event.Body), &body)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling message body: %w", err)
	}
	msgAttrs, ok := body.MessageAttributes[datadogSQSHeader]
	if !ok {
		return nil, errors.New("No Datadog trace context found")
	}
	if msgAttrs.Type != "Binary" {
		return nil, errors.New("Unsupported DataType in _datadog payload")
	}
	attr, err := base64.StdEncoding.DecodeString(string(msgAttrs.Value))
	if err != nil {
		return nil, fmt.Errorf("Error decoding binary: %w", err)
	}
	var carrier tracer.TextMapCarrier
	if err = json.Unmarshal(attr, &carrier); err != nil {
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
		return nil, errors.New("Could not unmarshal the invocation event payload")
	}
	return payload.Headers, nil
}

// headersCarrier returns the tracer.TextMapReader used to extract trace
// context from a Headers field of form map[string]string.
func headersCarrier(hdrs map[string]string) (tracer.TextMapReader, error) {
	return tracer.TextMapCarrier(hdrs), nil
}
