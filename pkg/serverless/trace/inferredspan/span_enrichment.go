// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

// EnrichInferredSpanWithAPIGatewayRESTEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a REST event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayRESTEvent(eventPayload events.APIGatewayProxyRequest) {
	log.Debug("Enriching an inferred span for a REST API Gateway")
	requestContext := eventPayload.RequestContext
	resource := fmt.Sprintf("%s %s", eventPayload.HTTPMethod, eventPayload.Path)
	httpurl := fmt.Sprintf("%s%s", requestContext.DomainName, eventPayload.Path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.DomainName
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		apiID:         requestContext.APIID,
		apiName:       requestContext.APIID,
		endpoint:      eventPayload.Path,
		httpURL:       httpurl,
		operationName: "aws.apigateway.rest",
		requestID:     requestContext.RequestID,
		resourceNames: resource,
		stage:         requestContext.Stage,
	}

	inferredSpan.IsAsync = eventPayload.Headers[invocationType] == "Event"
}

// EnrichInferredSpanWithAPIGatewayHTTPEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a HTTP event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayHTTPEvent(eventPayload events.APIGatewayV2HTTPRequest) {
	log.Debug("Enriching an inferred span for a HTTP API Gateway")
	requestContext := eventPayload.RequestContext
	http := requestContext.HTTP
	path := eventPayload.RequestContext.HTTP.Path
	resource := fmt.Sprintf("%s %s", http.Method, path)
	httpurl := fmt.Sprintf("%s%s", requestContext.DomainName, path)
	startTime := calculateStartTime(requestContext.TimeEpoch)

	inferredSpan.Span.Name = "aws.httpapi"
	inferredSpan.Span.Service = requestContext.DomainName
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		endpoint:      path,
		httpURL:       httpurl,
		httpMethod:    http.Method,
		httpProtocol:  http.Protocol,
		httpSourceIP:  http.SourceIP,
		httpUserAgent: http.UserAgent,
		operationName: "aws.httpapi",
		requestID:     requestContext.RequestID,
		resourceNames: resource,
	}

	inferredSpan.IsAsync = eventPayload.Headers[invocationType] == "Event"
}

// EnrichInferredSpanWithAPIGatewayWebsocketEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Websocket event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayWebsocketEvent(eventPayload events.APIGatewayWebsocketProxyRequest) {
	log.Debug("Enriching an inferred span for a Websocket API Gateway")
	requestContext := eventPayload.RequestContext
	routeKey := requestContext.RouteKey
	httpurl := fmt.Sprintf("%s%s", requestContext.DomainName, routeKey)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway.websocket"
	inferredSpan.Span.Service = requestContext.DomainName
	inferredSpan.Span.Resource = routeKey
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		apiID:            requestContext.APIID,
		apiName:          requestContext.APIID,
		connectionID:     requestContext.ConnectionID,
		endpoint:         routeKey,
		eventType:        requestContext.EventType,
		httpURL:          httpurl,
		messageDirection: requestContext.MessageDirection,
		operationName:    "aws.apigateway.websocket",
		requestID:        requestContext.RequestID,
		resourceNames:    routeKey,
		stage:            requestContext.Stage,
	}

	inferredSpan.IsAsync = eventPayload.Headers[invocationType] == "Event"
}

func (inferredSpan *InferredSpan) EnrichInferredSpanWithSNSEvent(eventPayload events.SNSEvent) {
	eventRecord := eventPayload.Records[0]
	snsMessage := eventRecord.SNS
	splitArn := strings.Split(snsMessage.TopicArn, ":")
	topicNameValue := splitArn[len(splitArn)-1]
	startTime := snsMessage.Timestamp.UnixNano()

	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.sns"
	inferredSpan.Span.Service = sns
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Resource = topicNameValue
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName: "aws.sns",
		resourceNames: topicNameValue,
		topicName:     topicNameValue,
		topicARN:      snsMessage.TopicArn,
		messageID:     snsMessage.MessageID,
		metadataType:  snsMessage.Type,
	}

	//Subject not available in SNS => SQS scenario
	if snsMessage.Subject != "" {
		inferredSpan.Span.Meta[subject] = snsMessage.Subject
	}
}

// CalculateStartTime converts AWS event timeEpochs to nanoseconds
func calculateStartTime(epoch int64) int64 {
	return epoch * 1e6
}

// formatISOStartTime converts ISO timestamps and returns
// a Unix timestamp in nanoseconds
func formatISOStartTime(isotime string) int64 {
	layout := "2006-01-02T15:04:05.000Z"
	startTime, err := time.Parse(layout, isotime)
	if err != nil {
		log.Debugf("Error parsing ISO time %s, failing with: %s", isotime, err)
		return 0
	}
	return startTime.UnixNano()
}
