// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"regexp"

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
	panic("not called")
}

func extractTraceContextFromSNSSQSEvent(firstRecord events.SQSMessage) *rawTraceContext {
	panic("not called")
}

func extractTraceContextFromDatadogHeader(ddPayloadValue events.SQSMessageAttribute) *rawTraceContext {
	panic("not called")
}

func extractTraceContextfromAWSTraceHeader(value string) *rawTraceContext {
	panic("not called")
}

func convertRawTraceContext(rawTrace *rawTraceContext) *convertedTraceContext {
	panic("not called")
}
