// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package trigger

// AWSEventType corresponds to the various event triggers
// we get from AWS
type AWSEventType int

const (
	// Unknown describes an unknown event type
	Unknown AWSEventType = iota

	// APIGatewayEvent describes an event from classic AWS API Gateways
	APIGatewayEvent

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
)

// eventParseFunc defines the signature of AWS event parsing functions
type eventParseFunc func(map[string]any) bool

type checker struct {
	check eventParseFunc
	typ   AWSEventType
}

var (
	unknownChecker = new(checker)
	// TODO: refactor to store the last event type on the execution context
	// instead of as a global
	lastEventChecker = unknownChecker
	eventCheckers    = []*checker{
		{isAPIGatewayEvent, APIGatewayEvent},
		{isAPIGatewayV2Event, APIGatewayV2Event},
		{isAPIGatewayWebsocketEvent, APIGatewayWebsocketEvent},
		{isAPIGatewayLambdaAuthorizerTokenEvent, APIGatewayLambdaAuthorizerTokenEvent},
		{isAPIGatewayLambdaAuthorizerRequestParametersEvent, APIGatewayLambdaAuthorizerRequestParametersEvent},
		{isALBEvent, ALBEvent},
		{isCloudFrontRequestEvent, CloudFrontRequestEvent},
		{isCloudwatchEvent, CloudWatchEvent},
		{isCloudwatchLogsEvent, CloudWatchLogsEvent},
		{isDynamoDBStreamEvent, DynamoDBStreamEvent},
		{isKinesisStreamEvent, KinesisStreamEvent},
		{isS3Event, S3Event},
		{isSNSEvent, SNSEvent},
		{isSNSSQSEvent, SNSSQSEvent},
		{isSQSEvent, SQSEvent},
		{isAppSyncResolverEvent, AppSyncResolverEvent},
		{isEventBridgeEvent, EventBridgeEvent},
		{isLambdaFunctionURLEvent, LambdaFunctionURLEvent},
		// Ultimately check this is a Kong API Gateway event as a last resort.
		// This is because Kong API Gateway events are a subset of API Gateway events
		// as of https://github.com/Kong/kong/blob/348c980/kong/plugins/aws-lambda/request-util.lua#L248-L260
		{isKongAPIGatewayEvent, APIGatewayEvent},
	}
)

// GetEventType takes in a payload string and returns an AWSEventType
// that matches the input payload. Returns `Unknown` if a payload could not be
// matched to an event.
func GetEventType(payload map[string]any) AWSEventType {
	panic("not called")
}

// Unmarshal unmarshals a payload string into a generic interface
func Unmarshal(payload []byte) (map[string]any, error) {
	panic("not called")
}

func isAPIGatewayEvent(event map[string]any) bool {
	panic("not called")
}

func isAPIGatewayV2Event(event map[string]any) bool {
	panic("not called")
}

// Kong API Gateway events are regular API Gateway events with a few missing
// fields (cf. https://github.com/Kong/kong/blob/348c980/kong/plugins/aws-lambda/request-util.lua#L248-L260)
// As a result, this function must be called after isAPIGatewayEvent() and
// related API Gateway event payload checks. It returns true when httpmethod and
// resource are present.
func isKongAPIGatewayEvent(event map[string]any) bool {
	panic("not called")
}

func isAPIGatewayWebsocketEvent(event map[string]any) bool {
	panic("not called")
}

func isAPIGatewayLambdaAuthorizerTokenEvent(event map[string]any) bool {
	panic("not called")
}

func isAPIGatewayLambdaAuthorizerRequestParametersEvent(event map[string]any) bool {
	panic("not called")
}

func isALBEvent(event map[string]any) bool {
	panic("not called")
}

func isCloudwatchEvent(event map[string]any) bool {
	panic("not called")
}

func isCloudwatchLogsEvent(event map[string]any) bool {
	panic("not called")
}

func isCloudFrontRequestEvent(event map[string]any) bool {
	panic("not called")
}

func isDynamoDBStreamEvent(event map[string]any) bool {
	panic("not called")
}

func isKinesisStreamEvent(event map[string]any) bool {
	panic("not called")
}

func isS3Event(event map[string]any) bool {
	panic("not called")
}

func isSNSEvent(event map[string]any) bool {
	panic("not called")
}

func isSQSEvent(event map[string]any) bool {
	panic("not called")
}

func isSNSSQSEvent(event map[string]any) bool {
	panic("not called")
}

func isAppSyncResolverEvent(event map[string]any) bool {
	panic("not called")
}

func isEventBridgeEvent(event map[string]any) bool {
	panic("not called")
}

func isLambdaFunctionURLEvent(event map[string]any) bool {
	panic("not called")
}

func eventRecordsKeyExists(event map[string]any, key string) bool {
	panic("not called")
}

func eventRecordsKeyEquals(event map[string]any, key string, val string) bool {
	panic("not called")
}

func (et AWSEventType) String() string {
	panic("not called")
}
