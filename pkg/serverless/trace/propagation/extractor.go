// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package propagation

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"

	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TextMapCarrier allows the use of a regular map[string]string as both TextMapWriter
// and TextMapReader, making it compatible with the provided Propagator.
type TextMapCarrier map[string]string

var _ ddtrace.TextMapWriter = (*TextMapCarrier)(nil)
var _ ddtrace.TextMapReader = (*TextMapCarrier)(nil)

// Set implements TextMapWriter.
func (c TextMapCarrier) Set(key, val string) {
	c[key] = val
}

// ForeachKey conforms to the TextMapReader interface.
func (c TextMapCarrier) ForeachKey(handler func(key, val string) error) error {
	for k, v := range c {
		if err := handler(k, v); err != nil {
			return err
		}
	}
	return nil
}

const (
	defaultSamplingPriority sampler.SamplingPriority = sampler.PriorityNone

	ddTraceIDHeader          = "x-datadog-trace-id"
	ddParentIDHeader         = "x-datadog-parent-id"
	ddSpanIDHeader           = "x-datadog-span-id"
	ddSamplingPriorityHeader = "x-datadog-sampling-priority"
	ddInvocationErrorHeader  = "x-datadog-invocation-error"
)

var (
	errorUnsupportedExtractionType = errors.New("Unsupported event type for trace context extraction")
	errorNoContextFound            = errors.New("No trace context found")
	errorNoSQSRecordFound          = errors.New("No sqs message records found for trace context extraction")
	errorNoSNSRecordFound          = errors.New("No sns message records found for trace context extraction")
	errorNoTraceIDFound            = errors.New("No trace ID found")
	errorNoParentIDFound           = errors.New("No parent ID found")
)

// Extractor inserts trace context into and extracts trace context out of
// different types.
type Extractor struct {
	propagator ddtrace.Propagator
}

// TraceContext stores the propagated trace context values.
type TraceContext struct {
	TraceID          uint64
	ParentID         uint64
	SamplingPriority sampler.SamplingPriority
}

// TraceContextExtended stores the propagated trace context values plus other
// non-standard header values.
type TraceContextExtended struct {
	*TraceContext
	SpanID          uint64
	InvocationError bool
}

// Extract looks in the given events one by one and returns once a proper trace
// context is found.
func (e Extractor) Extract(events ...interface{}) (*TraceContext, error) {
	for _, event := range events {
		if tc, err := e.extract(event); err == nil {
			return tc, nil
		}
	}
	return nil, errorNoContextFound
}

// extract uses dd-trace-go's Propagator type to extract trace context from the
// given event.
func (e Extractor) extract(event interface{}) (*TraceContext, error) {
	return nil, fmt.Errorf("ddtrace.Propagator is not available")

	// var carrier ddtrace.TextMapReader
	// var err error

	// switch ev := event.(type) {
	// case []byte:
	// 	carrier, err = rawPayloadCarrier(ev)
	// case events.SQSEvent:
	// 	// look for context in just the first message
	// 	if len(ev.Records) > 0 {
	// 		return e.extract(ev.Records[0])
	// 	}
	// 	return nil, errorNoSQSRecordFound
	// case events.SQSMessage:
	// 	if attr, ok := ev.Attributes[awsTraceHeader]; ok {
	// 		if tc, err := extractTraceContextfromAWSTraceHeader(attr); err == nil {
	// 			// Return early if AWSTraceHeader contains trace context
	// 			return tc, nil
	// 		}
	// 	}
	// 	carrier, err = sqsMessageCarrier(ev)
	// case events.SNSEvent:
	// 	// look for context in just the first message
	// 	if len(ev.Records) > 0 {
	// 		return e.extract(ev.Records[0].SNS)
	// 	}
	// 	return nil, errorNoSNSRecordFound
	// case events.SNSEntity:
	// 	carrier, err = snsEntityCarrier(ev)
	// case events.APIGatewayProxyRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// case events.APIGatewayV2HTTPRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// case events.APIGatewayWebsocketProxyRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// case events.APIGatewayCustomAuthorizerRequestTypeRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// case events.ALBTargetGroupRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// case events.LambdaFunctionURLRequest:
	// 	carrier, err = headersCarrier(ev.Headers)
	// default:
	// 	err = errorUnsupportedExtractionType
	// }

	// if err != nil {
	// 	return nil, err
	// }
	// if e.propagator == nil {
	// 	e.propagator = tracer.NewPropagator(nil)
	// }
	// sc, err := e.propagator.Extract(carrier)
	// if err != nil {
	// 	return nil, err
	// }
	// return &TraceContext{
	// 	TraceID:          sc.TraceID(),
	// 	ParentID:         sc.SpanID(),
	// 	SamplingPriority: getSamplingPriority(sc),
	// }, nil
}

// ExtractFromLayer is used for extracting context from the request headers
// sent from a tracing layer. Currently, only datadog style headers are
// extracted. If a trace id or parent id are not found, then the embedded
// *TraceContext will be nil.
func (e Extractor) ExtractFromLayer(hdr http.Header) *TraceContextExtended {
	tc, err := e.extractTraceContextFromLayer(hdr)
	if err != nil {
		log.Debugf("unable to find trace context in request headers: %s", err)
	}

	var spanID uint64
	if value, err := convertStrToUint64(hdr.Get(ddSpanIDHeader)); err == nil {
		log.Debugf("injecting spanId = %v", value)
		spanID = value
	}

	invocationError := hdr.Get(ddInvocationErrorHeader) == "true"

	return &TraceContextExtended{
		TraceContext:    tc,
		SpanID:          spanID,
		InvocationError: invocationError,
	}
}

func (e Extractor) extractTraceContextFromLayer(hdr http.Header) (*TraceContext, error) {
	var traceID uint64
	if value, err := convertStrToUint64(hdr.Get(ddTraceIDHeader)); err == nil {
		log.Debugf("injecting traceID = %v", value)
		traceID = value
	}
	if traceID == 0 {
		return nil, errorNoTraceIDFound
	}

	var parentID uint64
	if value, err := convertStrToUint64(hdr.Get(ddParentIDHeader)); err == nil {
		log.Debugf("injecting parentId = %v", value)
		parentID = value
	}
	if parentID == 0 {
		return nil, errorNoParentIDFound
	}

	samplingPriority := defaultSamplingPriority
	if value, err := strconv.ParseInt(hdr.Get(ddSamplingPriorityHeader), 10, 8); err == nil {
		log.Debugf("injecting samplingPriority = %v", value)
		samplingPriority = sampler.SamplingPriority(value)
	}

	return &TraceContext{
		TraceID:          traceID,
		ParentID:         parentID,
		SamplingPriority: samplingPriority,
	}, nil
}

// InjectToLayer is used for injecting context into the response headers sent
// to a tracing layer. Currently, only datadog style headers are injected.
func (e Extractor) InjectToLayer(tc *TraceContext, hdr http.Header) {
	if tc != nil {
		hdr.Set(ddTraceIDHeader, fmt.Sprintf("%v", tc.TraceID))
		hdr.Set(ddSamplingPriorityHeader, fmt.Sprintf("%v", tc.SamplingPriority))
	}
}

// getSamplingPriority searches the given ddtrace.SpanContext for sampling
// priority. Note that not all versions of ddtrace export the SamplingPriority
// method, therefore the interface check is required.
func getSamplingPriority(sc ddtrace.SpanContext) (priority sampler.SamplingPriority) {
	priority = defaultSamplingPriority
	if pc, ok := sc.(interface{ SamplingPriority() (int, bool) }); ok && pc != nil {
		if p, ok := pc.SamplingPriority(); ok {
			priority = sampler.SamplingPriority(p)
		}
	}
	return
}

// convertStrToUint64 converts a given string to uint64 optionally returning an
// error.
func convertStrToUint64(s string) (uint64, error) {
	num, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		log.Debugf("Error while converting %s, failing with : %s", s, err)
	}
	return num, err
}
