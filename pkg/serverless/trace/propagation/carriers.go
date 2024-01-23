// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package propagation manages propagation of trace context headers.
package propagation

import (
	"errors"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
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
	panic("not called")
}

// sqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessage type.
func sqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	panic("not called")
}

// sqsMessageAttrCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessageAttribute field on an events.SQSMessage
// type.
func sqsMessageAttrCarrier(attr events.SQSMessageAttribute) (tracer.TextMapReader, error) {
	panic("not called")
}

// snsBody is used to  unmarshal only required fields on events.SNSEntity
// types.
type snsBody struct {
	MessageAttributes map[string]interface{}
}

// snsSqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the body of an events.SQSMessage type.
func snsSqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	panic("not called")
}

// snsEntityCarrier returns the tracer.TextMapReader used to extract trace
// context from the attributes of an events.SNSEntity type.
func snsEntityCarrier(event events.SNSEntity) (tracer.TextMapReader, error) {
	panic("not called")
}

type invocationPayload struct {
	Headers tracer.TextMapCarrier `json:"headers"`
}

// rawPayloadCarrier returns the tracer.TextMapReader used to extract trace
// context from the raw json event payload.
func rawPayloadCarrier(rawPayload []byte) (tracer.TextMapReader, error) {
	panic("not called")
}

// headersCarrier returns the tracer.TextMapReader used to extract trace
// context from a Headers field of form map[string]string.
func headersCarrier(hdrs map[string]string) (tracer.TextMapReader, error) {
	panic("not called")
}
