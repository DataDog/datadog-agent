// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
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
	panic("not called")
}

// This function gets a snapshot of the current service mapping without modifying it.
//
//nolint:revive // TODO(SERV) Fix revive linter
func GetServiceMapping() map[string]string {
	panic("not called")
}

//nolint:revive // TODO(SERV) Fix revive linter
func DetermineServiceName(serviceMapping map[string]string, specificKey string, genericKey string, defaultValue string) string {
	panic("not called")
}

// EnrichInferredSpanWithAPIGatewayRESTEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a REST event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayRESTEvent(eventPayload events.APIGatewayProxyRequest) {
	panic("not called")
}

// EnrichInferredSpanWithAPIGatewayHTTPEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a HTTP event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayHTTPEvent(eventPayload events.APIGatewayV2HTTPRequest) {
	panic("not called")
}

// EnrichInferredSpanWithLambdaFunctionURLEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Lambda Function URL event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithLambdaFunctionURLEvent(eventPayload events.LambdaFunctionURLRequest) {
	panic("not called")
}

// EnrichInferredSpanWithAPIGatewayWebsocketEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Websocket event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithAPIGatewayWebsocketEvent(eventPayload events.APIGatewayWebsocketProxyRequest) {
	panic("not called")
}

// EnrichInferredSpanWithSNSEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an SNS event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithSNSEvent(eventPayload events.SNSEvent) {
	panic("not called")
}

// EnrichInferredSpanWithS3Event uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an S3 event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithS3Event(eventPayload events.S3Event) {
	panic("not called")
}

// EnrichInferredSpanWithSQSEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an SQS event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithSQSEvent(eventPayload events.SQSEvent) {
	panic("not called")
}

// EnrichInferredSpanWithEventBridgeEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from an EventBridge event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithEventBridgeEvent(eventPayload events.EventBridgeEvent) {
	panic("not called")
}

// EnrichInferredSpanWithKinesisEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a Kinesis event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithKinesisEvent(eventPayload events.KinesisEvent) {
	panic("not called")
}

// EnrichInferredSpanWithDynamoDBEvent uses the parsed event
// payload to enrich the current inferred span. It applies a
// specific set of data to the span expected from a DynamoDB event.
func (inferredSpan *InferredSpan) EnrichInferredSpanWithDynamoDBEvent(eventPayload events.DynamoDBEvent) {
	panic("not called")
}

// CalculateStartTime converts AWS event timeEpochs to nanoseconds
func calculateStartTime(epoch int64) int64 {
	panic("not called")
}

// formatISOStartTime converts ISO timestamps and returns
// a Unix timestamp in nanoseconds
func formatISOStartTime(isotime string) int64 {
	panic("not called")
}

func convertStringTimestamp(timestamp string) int64 {
	panic("not called")
}
