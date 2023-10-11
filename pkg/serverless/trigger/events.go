// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package trigger

import (
	jsonEncoder "encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/json"
)

// AWSEventType corresponds to the various event triggers
// we get from AWS
type AWSEventType int

const (
	// APIGatewayEvent describes an event from classic AWS API Gateways
	APIGatewayEvent AWSEventType = iota

	// APIGatewayV2Event describes an event from AWS API Gateways
	APIGatewayV2Event

	// APIGatewayWebsocketEvent describes an event from websocket AWS API Gateways
	APIGatewayWebsocketEvent

	// APIGatewayLambdaAuthorizerTokenEvent describes an event from a token-based API Gateway lambda authorizer
	APIGatewayLambdaAuthorizerTokenEvent

	// APIGatewayLambdaAuthorizerRequestParametersEvent describes an event from a request-parameters-based API Gateway lambda authorizer
	APIGatewayLambdaAuthorizerRequestParametersEvent

	// ALBEvent describes an event from application load balancers
	ALBEvent

	// CloudWatchEvent describes an event from Cloudwatch
	CloudWatchEvent

	// CloudWatchLogsEvent describes an event from Cloudwatch logs
	CloudWatchLogsEvent

	// CloudFrontRequestEvent describes an event from Cloudfront
	CloudFrontRequestEvent

	// DynamoDBStreamEvent describes an event from DynamoDB
	DynamoDBStreamEvent

	// KinesisStreamEvent describes an event from Kinesis
	KinesisStreamEvent

	// S3Event describes an event from S3
	S3Event

	// SNSEvent describes an event from SNS
	SNSEvent

	// SQSEvent describes an event from SQS
	SQSEvent

	// SNSSQSEvent describes an event from an SQS-wrapped SNS topic
	SNSSQSEvent

	// AppSyncResolverEvent describes an event from Appsync
	AppSyncResolverEvent

	// EventBridgeEvent describes an event from EventBridge
	EventBridgeEvent

	// LambdaFunctionURLEvent describes an event from an HTTP lambda function URL invocation
	LambdaFunctionURLEvent

	// Unknown describes an unknown event type
	Unknown
)

// eventParseFunc defines the signature of AWS event parsing functions
type eventParseFunc func(map[string]any) bool

// GetEventType takes in a payload string and returns an AWSEventType
// that matches the input payload. Returns `Unknown` if a payload could not be
// matched to an event.
func GetEventType(payload map[string]any) AWSEventType {
	if isAPIGatewayEvent(payload) {
		return APIGatewayEvent
	}

	if isAPIGatewayV2Event(payload) {
		return APIGatewayV2Event
	}

	if isAPIGatewayWebsocketEvent(payload) {
		return APIGatewayWebsocketEvent
	}

	if isAPIGatewayLambdaAuthorizerTokenEvent(payload) {
		return APIGatewayLambdaAuthorizerTokenEvent
	}

	if isAPIGatewayLambdaAuthorizerRequestParametersEvent(payload) {
		return APIGatewayLambdaAuthorizerRequestParametersEvent
	}

	if isALBEvent(payload) {
		return ALBEvent
	}

	if isCloudFrontRequestEvent(payload) {
		return CloudFrontRequestEvent
	}

	if isCloudwatchEvent(payload) {
		return CloudWatchEvent
	}

	if isCloudwatchLogsEvent(payload) {
		return CloudWatchLogsEvent
	}

	if isDynamoDBStreamEvent(payload) {
		return DynamoDBStreamEvent
	}

	if isKinesisStreamEvent(payload) {
		return KinesisStreamEvent
	}

	if isS3Event(payload) {
		return S3Event
	}

	if isSNSEvent(payload) {
		return SNSEvent
	}

	if isSNSSQSEvent(payload) {
		return SNSSQSEvent
	}

	if isSQSEvent(payload) {
		return SQSEvent
	}

	if isAppSyncResolverEvent(payload) {
		return AppSyncResolverEvent
	}

	if isEventBridgeEvent(payload) {
		return EventBridgeEvent
	}

	if isLambdaFunctionURLEvent(payload) {
		return LambdaFunctionURLEvent
	}

	return Unknown
}

// Unmarshal unmarshals a payload string into a generic interface
func Unmarshal(payload []byte) (map[string]any, error) {
	jsonPayload := make(map[string]any)
	if err := jsonEncoder.Unmarshal(payload, &jsonPayload); err != nil {
		return nil, err
	}
	return jsonPayload, nil
}

func isAPIGatewayEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "requestcontext") != nil &&
		json.GetNestedValue(event, "httpmethod") != nil &&
		json.GetNestedValue(event, "resource") != nil &&
		!isAPIGatewayLambdaAuthorizerRequestParametersEvent(event)
}

func isAPIGatewayV2Event(event map[string]any) bool {
	version, ok := json.GetNestedValue(event, "version").(string)
	if !ok {
		return false
	}
	domainName, ok := json.GetNestedValue(event, "requestcontext", "domainname").(string)
	if !ok {
		return false
	}
	return version == "2.0" &&
		json.GetNestedValue(event, "rawquerystring") != nil &&
		!strings.Contains(domainName, "lambda-url")
}

func isAPIGatewayWebsocketEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "requestcontext") != nil &&
		json.GetNestedValue(event, "requestcontext", "messagedirection") != nil
}

func isAPIGatewayLambdaAuthorizerTokenEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "type") == "token" &&
		json.GetNestedValue(event, "authorizationtoken") != nil &&
		json.GetNestedValue(event, "methodarn") != nil
}

func isAPIGatewayLambdaAuthorizerRequestParametersEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "type") == "request" &&
		json.GetNestedValue(event, "methodarn") != nil &&
		json.GetNestedValue(event, "headers") != nil &&
		json.GetNestedValue(event, "querystringparameters") != nil &&
		json.GetNestedValue(event, "requestcontext", "apiid") != nil
}

func isALBEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "requestcontext", "elb") != nil
}

func isCloudwatchEvent(event map[string]any) bool {
	source, ok := json.GetNestedValue(event, "source").(string)
	return ok && source == "aws.events"
}

func isCloudwatchLogsEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "awslogs") != nil
}

func isCloudFrontRequestEvent(event map[string]any) bool {
	return eventRecordsKeyExists(event, "cf")
}

func isDynamoDBStreamEvent(event map[string]any) bool {
	return eventRecordsKeyExists(event, "dynamodb")
}

func isKinesisStreamEvent(event map[string]any) bool {
	return eventRecordsKeyExists(event, "kinesis")
}

func isS3Event(event map[string]any) bool {
	return eventRecordsKeyExists(event, "s3")
}

func isSNSEvent(event map[string]any) bool {
	return eventRecordsKeyExists(event, "sns")
}

func isSQSEvent(event map[string]any) bool {
	return eventRecordsKeyEquals(event, "eventsource", "aws:sqs")
}

func isSNSSQSEvent(event map[string]any) bool {
	if !eventRecordsKeyEquals(event, "eventsource", "aws:sqs") {
		return false
	}
	messageType, ok := json.GetNestedValue(event, "body", "type").(string)
	if !ok {
		return false
	}

	topicArn, ok := json.GetNestedValue(event, "body", "topicarn").(string)
	if !ok {
		return false
	}

	return messageType == "notification" && topicArn != ""
}

func isAppSyncResolverEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "info", "selectionsetgraphql") != nil
}

func isEventBridgeEvent(event map[string]any) bool {
	return json.GetNestedValue(event, "detail-type") != nil && json.GetNestedValue(event, "source") != "aws.events"
}

func isLambdaFunctionURLEvent(event map[string]any) bool {
	lambdaURL, ok := json.GetNestedValue(event, "requestcontext", "domainname").(string)
	if !ok {
		return false
	}
	return strings.Contains(lambdaURL, "lambda-url")
}

func eventRecordsKeyExists(event map[string]any, key string) bool {
	records, ok := json.GetNestedValue(event, "records").([]interface{})
	if !ok {
		return false
	}
	if len(records) > 0 {
		if records[0].(map[string]any)[key] != nil {
			return true
		}
	}
	return false
}

func eventRecordsKeyEquals(event map[string]any, key string, val string) bool {
	records, ok := json.GetNestedValue(event, "records").([]interface{})
	if !ok {
		return false
	}
	if len(records) > 0 {
		if mapVal := records[0].(map[string]any)[key]; mapVal != nil {
			return mapVal == val
		}
	}
	return false
}
