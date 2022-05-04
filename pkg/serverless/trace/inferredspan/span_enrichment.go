// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EnrichInferredSpanWithAPIGatewayRESTEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a REST event.
func EnrichInferredSpanWithAPIGatewayRESTEvent(attributes EventKeys, inferredSpan InferredSpan) {

	log.Debug("Enriching an inferred span for a REST API Gateway")
	requestContext := attributes.RequestContext
	resource := fmt.Sprintf("%s %s", attributes.HTTPMethod, attributes.Path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, attributes.Path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		APIID:         requestContext.APIID,
		APIName:       requestContext.APIID,
		Endpoint:      attributes.Path,
		HTTPURL:       httpurl,
		OperationName: "aws.apigateway.rest",
		RequestID:     requestContext.RequestID,
		ResourceNames: resource,
		Stage:         requestContext.Stage,
	}

	inferredSpan.IsAsync = isAsyncEvent(attributes)
}

// EnrichInferredSpanWithAPIGatewayHTTPEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a HTTP event.
func EnrichInferredSpanWithAPIGatewayHTTPEvent(attributes EventKeys, inferredSpan InferredSpan) {
	log.Debug("Enriching an inferred span for a HTTP API Gateway")
	requestContext := attributes.RequestContext
	http := requestContext.HTTP
	path := requestContext.RawPath
	resource := fmt.Sprintf("%s %s", http.Method, path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, path)
	startTime := calculateStartTime(requestContext.TimeEpoch)

	inferredSpan.Span.Name = "aws.httpapi"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		Endpoint:      path,
		HTTPURL:       httpurl,
		HTTPMethod:    http.Method,
		HTTPProtocol:  http.Protocol,
		HTTPSourceIP:  http.SourceIP,
		HTTPUserAgent: http.UserAgent,
		OperationName: "aws.httpapi",
		RequestID:     requestContext.RequestID,
		ResourceNames: resource,
	}

	inferredSpan.IsAsync = isAsyncEvent(attributes)
}

// EnrichInferredSpanWithAPIGatewayWebsocketEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Websocket event.
func EnrichInferredSpanWithAPIGatewayWebsocketEvent(attributes EventKeys, inferredSpan InferredSpan) {
	log.Debug("Enriching an inferred span for a Websocket API Gateway")
	requestContext := attributes.RequestContext
	endpoint := requestContext.RouteKey
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, endpoint)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway.websocket"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = endpoint
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		APIID:            requestContext.APIID,
		APIName:          requestContext.APIID,
		ConnectionID:     requestContext.ConnectionID,
		Endpoint:         endpoint,
		EventType:        requestContext.EventType,
		HTTPURL:          httpurl,
		MessageDirection: requestContext.MessageDirection,
		OperationName:    "aws.apigateway.websocket",
		RequestID:        requestContext.RequestID,
		ResourceNames:    endpoint,
		Stage:            requestContext.Stage,
	}

	inferredSpan.IsAsync = isAsyncEvent(attributes)
}

func isAsyncEvent(attributes EventKeys) bool {
	return attributes.Headers.InvocationType == "Event"
}

// CalculateStartTime converts AWS event timeEpochs to nanoseconds
func calculateStartTime(epoch int64) int64 {
	return (epoch / 1000) * 1e9
}
