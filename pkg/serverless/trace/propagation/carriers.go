// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package propagation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	awsTraceHeader   = "AWSTraceHeader"
	datadogSQSHeader = "_datadog"

	rootPrefix    = "Root="
	parentPrefix  = "Parent="
	rootPadding   = len(rootPrefix + "1-00000000-00000000")
	parentPadding = len(parentPrefix)
)

var rootRegex = regexp.MustCompile("Root=1-[0-9a-fA-F]{8}-00000000[0-9a-fA-F]{16}")

func extractTraceContextfromAWSTraceHeader(value string) (*TraceContext, error) {
	if !rootRegex.MatchString(value) {
		return nil, errors.New("AWSTraceHeader does not match expected regex")
	}
	var (
		startPart int
		traceID   string
		parentID  string
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
		}
		if traceID != "" && parentID != "" {
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
	if tc.TraceID == 0 || tc.ParentID == 0 {
		return nil, errors.New("AWSTraceHeader does not contain trace ID and parent ID")
	}
	return tc, nil
}

func sqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	if attr, ok := event.MessageAttributes[datadogSQSHeader]; ok {
		return sqsMessageAttrCarrier(attr)
	}
	return snsSqsMessageCarrier(event)
}

func sqsMessageAttrCarrier(attr events.SQSMessageAttribute) (tracer.TextMapReader, error) {
	var carrier tracer.TextMapCarrier
	if attr.DataType != "String" {
		return nil, errors.New("Unsupported DataType in _datadog payload")
	}
	if attr.StringValue == nil {
		return nil, errors.New("String value not found in _datadog payload")
	}
	if err := json.Unmarshal([]byte(*attr.StringValue), &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling payload value: %w", err)
	}
	return carrier, nil
}

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

func rawPayloadCarrier(rawPayload []byte) (tracer.TextMapReader, error) {
	var payload invocationPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, errors.New("Could not unmarshal the invocation event payload")
	}
	return payload.Headers, nil
}

func headersCarrier(hdrs map[string]string) (tracer.TextMapReader, error) {
	return tracer.TextMapCarrier(hdrs), nil
}
