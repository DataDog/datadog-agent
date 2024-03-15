// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Define and initialize serviceMapping as a global variable.
var serviceMapping map[string]string

//nolint:revive // TODO(SERV) Fix revive linter
func CreateServiceMapping(val string) map[string]string {
	newServiceMapping := make(map[string]string)

	for _, entry := range strings.Split(val, ",") {
		parts := strings.Split(entry, ":")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" && value != "" && key != value {
				newServiceMapping[key] = value
			}
		}
	}
	return newServiceMapping
}

func init() {
	serviceMappingStr := config.Datadog.GetString("serverless.service_mapping")
	serviceMapping = CreateServiceMapping(serviceMappingStr)
}

// SetServiceMapping sets the serviceMapping global variable, primarily for tests.
func SetServiceMapping(newServiceMapping map[string]string) {
	serviceMapping = newServiceMapping
}

// This function gets a snapshot of the current service mapping without modifying it.
//
//nolint:revive // TODO(SERV) Fix revive linter
func GetServiceMapping() map[string]string {
	return serviceMapping
}

//nolint:revive // TODO(SERV) Fix revive linter
func DetermineServiceName(serviceMapping map[string]string, specificKey string, genericKey string, defaultValue string) string {
	var serviceName string
	if val, ok := serviceMapping[specificKey]; ok {
		serviceName = val
	} else if val, ok := serviceMapping[genericKey]; ok {
		serviceName = val
	} else {
		serviceName = defaultValue
	}
	return serviceName
}

// EnrichInferredSpanWithAPIGatewayRESTEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a REST event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayRESTEvent(eventPayload events.APIGatewayProxyRequest) {
	log.Debug("Enriching an inferred span for a REST API Gateway")
	requestContext := eventPayload.RequestContext
	resource := fmt.Sprintf("%s %s", eventPayload.HTTPMethod, eventPayload.Path)
	domain := requestContext.DomainName
	httpurl := fmt.Sprintf("%s%s", domain, eventPayload.Path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)
	//nolint:revive // TODO(SERV) Fix revive linter
	apiId := requestContext.APIID
	serviceName := DetermineServiceName(serviceMapping, apiId, "lambda_api_gateway", domain)
	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = serviceName
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
	domainName := requestContext.DomainName
	httpurl := fmt.Sprintf("%s%s", domainName, path)
	startTime := calculateStartTime(requestContext.TimeEpoch)
	//nolint:revive // TODO(SERV) Fix revive linter
	apiId := requestContext.APIID
	serviceName := DetermineServiceName(serviceMapping, apiId, "lambda_api_gateway", domainName)
	inferredSpan.Span.Name = "aws.httpapi"
	inferredSpan.Span.Service = serviceName
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

// EnrichInferredSpanWithLambdaFunctionURLEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Lambda Function URL event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithLambdaFunctionURLEvent(eventPayload events.LambdaFunctionURLRequest) {
	log.Debug("Enriching an inferred span for a Lambda Function URL")
	requestContext := eventPayload.RequestContext
	http := requestContext.HTTP
	path := eventPayload.RequestContext.HTTP.Path
	resource := fmt.Sprintf("%s %s", http.Method, path)
	domainName := requestContext.DomainName
	httpurl := fmt.Sprintf("%s%s", domainName, path)
	startTime := calculateStartTime(requestContext.TimeEpoch)
	apiID := requestContext.APIID
	serviceName := DetermineServiceName(serviceMapping, apiID, "lambda_url", domainName)
	inferredSpan.Span.Name = "aws.lambda.url"
	inferredSpan.Span.Service = serviceName
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
		operationName: "aws.lambda.url",
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
	//nolint:revive // TODO(SERV) Fix revive linter
	apiId := requestContext.APIID
	serviceName := DetermineServiceName(serviceMapping, apiId, "lambda_api_gateway", requestContext.DomainName)
	inferredSpan.Span.Name = "aws.apigateway.websocket"
	inferredSpan.Span.Service = serviceName
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

// EnrichInferredSpanWithSNSEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an SNS event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithSNSEvent(eventPayload events.SNSEvent) {
	eventRecord := eventPayload.Records[0]
	snsMessage := eventRecord.SNS
	splitArn := strings.Split(snsMessage.TopicArn, ":")
	topicNameValue := splitArn[len(splitArn)-1]
	serviceName := DetermineServiceName(serviceMapping, topicNameValue, "lambda_sns", "sns")
	startTime := snsMessage.Timestamp.UnixNano()

	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.sns"
	inferredSpan.Span.Service = serviceName
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

// EnrichInferredSpanWithS3Event uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an S3 event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithS3Event(eventPayload events.S3Event) {
	eventRecord := eventPayload.Records[0]
	bucketNameVar := eventRecord.S3.Bucket.Name
	serviceName := DetermineServiceName(serviceMapping, bucketNameVar, "lambda_s3", "s3")
	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.s3"
	inferredSpan.Span.Service = serviceName
	inferredSpan.Span.Start = eventRecord.EventTime.UnixNano()
	inferredSpan.Span.Resource = bucketNameVar
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName: "aws.s3",
		resourceNames: bucketNameVar,
		eventName:     eventRecord.EventName,
		bucketName:    bucketNameVar,
		bucketARN:     eventRecord.S3.Bucket.Arn,
		objectKey:     eventRecord.S3.Object.Key,
		objectSize:    strconv.FormatInt(eventRecord.S3.Object.Size, 10),
		objectETag:    eventRecord.S3.Object.ETag,
	}
}

// EnrichInferredSpanWithSQSEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an SQS event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithSQSEvent(eventPayload events.SQSEvent) {
	eventRecord := eventPayload.Records[0]
	splitArn := strings.Split(eventRecord.EventSourceARN, ":")
	parsedQueueName := splitArn[len(splitArn)-1]
	startTime := calculateStartTime(convertStringTimestamp(eventRecord.Attributes[sentTimestamp]))
	serviceName := DetermineServiceName(serviceMapping, parsedQueueName, "lambda_sqs", "sqs")

	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.sqs"
	inferredSpan.Span.Service = serviceName
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Resource = parsedQueueName
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName:  "aws.sqs",
		resourceNames:  parsedQueueName,
		queueName:      parsedQueueName,
		eventSourceArn: eventRecord.EventSourceARN,
		receiptHandle:  eventRecord.ReceiptHandle,
		senderID:       eventRecord.Attributes["SenderId"],
	}
}

// EnrichInferredSpanWithEventBridgeEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an EventBridge event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithEventBridgeEvent(eventPayload events.EventBridgeEvent) {
	source := eventPayload.Source
	serviceName := DetermineServiceName(serviceMapping, source, "lambda_eventbridge", "eventbridge")
	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.eventbridge"
	inferredSpan.Span.Service = serviceName
	inferredSpan.Span.Start = formatISOStartTime(eventPayload.StartTime)
	inferredSpan.Span.Resource = source
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName: "aws.eventbridge",
		resourceNames: source,
		detailType:    eventPayload.DetailType,
	}
}

// EnrichInferredSpanWithKinesisEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Kinesis event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithKinesisEvent(eventPayload events.KinesisEvent) {
	eventRecord := eventPayload.Records[0]
	eventSourceARN := eventRecord.EventSourceArn
	parts := strings.Split(eventSourceARN, "/")
	parsedStreamName := parts[len(parts)-1]
	parsedShardID := strings.Split(eventRecord.EventID, ":")[0]
	serviceName := DetermineServiceName(serviceMapping, parsedStreamName, "lambda_kinesis", "kinesis")
	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.kinesis"
	inferredSpan.Span.Service = serviceName
	inferredSpan.Span.Start = eventRecord.Kinesis.ApproximateArrivalTimestamp.UnixNano()
	inferredSpan.Span.Resource = parsedStreamName
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName:  "aws.kinesis",
		resourceNames:  parsedStreamName,
		streamName:     parsedStreamName,
		shardID:        parsedShardID,
		eventSourceArn: eventSourceARN,
		eventID:        eventRecord.EventID,
		eventName:      eventRecord.EventName,
		eventVersion:   eventRecord.EventVersion,
		partitionKey:   eventRecord.Kinesis.PartitionKey,
	}
}

// EnrichInferredSpanWithDynamoDBEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a DynamoDB event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithDynamoDBEvent(eventPayload events.DynamoDBEvent) {
	eventRecord := eventPayload.Records[0]
	parsedTableName := strings.Split(eventRecord.EventSourceArn, "/")[1]
	eventMessage := eventRecord.Change
	serviceName := DetermineServiceName(serviceMapping, parsedTableName, "lambda_dynamodb", "dynamodb")
	inferredSpan.IsAsync = true
	inferredSpan.Span.Name = "aws.dynamodb"
	inferredSpan.Span.Service = serviceName
	inferredSpan.Span.Start = eventMessage.ApproximateCreationDateTime.UnixNano()
	inferredSpan.Span.Resource = parsedTableName
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Meta = map[string]string{
		operationName:  "aws.dynamodb",
		resourceNames:  parsedTableName,
		tableName:      parsedTableName,
		eventSourceArn: eventRecord.EventSourceArn,
		eventID:        eventRecord.EventID,
		eventName:      eventRecord.EventName,
		eventVersion:   eventRecord.EventVersion,
		streamViewType: eventRecord.Change.StreamViewType,
		sizeBytes:      strconv.FormatInt(eventRecord.Change.SizeBytes, 10),
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

func convertStringTimestamp(timestamp string) int64 {
	t, err := strconv.ParseInt(timestamp, 0, 64)
	if err != nil {
		log.Debug("Could not parse timestamp from string")
		return 0
	}
	return t
}
