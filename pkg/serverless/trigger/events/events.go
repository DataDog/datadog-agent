// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package events provides a series of drop in replacements for
// "github.com/aws/aws-lambda-go/events".  Using these types for json
// unmarshalling event payloads provides huge reduction in processing time.
// This means fewer map/slice allocations since only the fields which we will
// use will be unmarshalled.
package events

import (
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// APIGatewayProxyRequest mirrors events.APIGatewayProxyRequest type, removing
// unused fields.
type APIGatewayProxyRequest struct {
	Resource       string
	Path           string
	HTTPMethod     string
	Headers        map[string]string
	RequestContext APIGatewayProxyRequestContext
}

// APIGatewayProxyRequestContext mirrors events.APIGatewayProxyRequestContext
// type, removing unused fields.
type APIGatewayProxyRequestContext struct {
	Stage            string
	DomainName       string
	RequestID        string
	Path             string
	HTTPMethod       string
	RequestTimeEpoch int64
	APIID            string
}

// APIGatewayV2HTTPRequest mirrors events.APIGatewayV2HTTPRequest type,
// removing unused fields.
type APIGatewayV2HTTPRequest struct {
	RouteKey       string
	Headers        map[string]string
	RequestContext APIGatewayV2HTTPRequestContext
}

// APIGatewayV2HTTPRequestContext mirrors events.APIGatewayV2HTTPRequestContext
// type, removing unused fields.
type APIGatewayV2HTTPRequestContext struct {
	Stage      string
	RequestID  string
	APIID      string
	DomainName string
	TimeEpoch  int64
	HTTP       APIGatewayV2HTTPRequestContextHTTPDescription
}

// APIGatewayV2HTTPRequestContextHTTPDescription mirrors
// events.APIGatewayV2HTTPRequestContextHTTPDescription type, removing unused
// fields.
type APIGatewayV2HTTPRequestContextHTTPDescription struct {
	Method    string
	Path      string
	Protocol  string
	SourceIP  string
	UserAgent string
}

// APIGatewayWebsocketProxyRequest mirrors
// events.APIGatewayWebsocketProxyRequest type, removing unused fields.
type APIGatewayWebsocketProxyRequest struct {
	Headers        map[string]string
	RequestContext APIGatewayWebsocketProxyRequestContext
}

// APIGatewayWebsocketProxyRequestContext mirrors
// events.APIGatewayWebsocketProxyRequestContext type, removing unused fields.
type APIGatewayWebsocketProxyRequestContext struct {
	Stage            string
	RequestID        string
	APIID            string
	ConnectionID     string
	DomainName       string
	EventType        string
	MessageDirection string
	RequestTimeEpoch int64
	RouteKey         string
}

// APIGatewayCustomAuthorizerRequest mirrors
// events.APIGatewayCustomAuthorizerRequest type, removing unused fields.
type APIGatewayCustomAuthorizerRequest struct {
	Type               string
	AuthorizationToken string
	MethodArn          string
}

// APIGatewayCustomAuthorizerRequestTypeRequest mirrors
// events.APIGatewayCustomAuthorizerRequestTypeRequest type, removing unused
// fields.
type APIGatewayCustomAuthorizerRequestTypeRequest struct {
	MethodArn      string
	Resource       string
	HTTPMethod     string
	Headers        map[string]string
	RequestContext APIGatewayCustomAuthorizerRequestTypeRequestContext
}

// APIGatewayCustomAuthorizerRequestTypeRequestContext mirrors
// events.APIGatewayCustomAuthorizerRequestTypeRequestContext type, removing
// unused fields.
type APIGatewayCustomAuthorizerRequestTypeRequestContext struct {
	Path string
}

// ALBTargetGroupRequest mirrors events.ALBTargetGroupRequest type, removing
// unused fields.
type ALBTargetGroupRequest struct {
	HTTPMethod     string
	Path           string
	Headers        map[string]string
	RequestContext ALBTargetGroupRequestContext
}

// ALBTargetGroupRequestContext mirrors events.ALBTargetGroupRequestContext
// type, removing unused fields.
type ALBTargetGroupRequestContext struct {
	ELB ELBContext
}

// ELBContext mirrors events.ELBContext type, removing unused fields.
type ELBContext struct {
	TargetGroupArn string
}

// CloudWatchEvent mirrors events.CloudWatchEvent type, removing unused fields.
type CloudWatchEvent struct {
	Resources []string
}

// CloudwatchLogsEvent mirrors events.CloudwatchLogsEvent type, removing unused
// fields.
type CloudwatchLogsEvent struct {
	AWSLogs CloudwatchLogsRawData
}

// CloudwatchLogsRawData mirrors events.CloudwatchLogsRawData type, removing
// unused fields.
type CloudwatchLogsRawData struct {
	Data string
}

// Parse returns a struct representing a usable CloudwatchLogs event
func (c CloudwatchLogsRawData) Parse() (d CloudwatchLogsData, err error) {
	panic("not called")
}

// CloudwatchLogsData mirrors events.CloudwatchLogsData type, removing unused
// fields.
type CloudwatchLogsData struct {
	LogGroup string
}

// DynamoDBEvent mirrors events.DynamoDBEvent type, removing unused fields.
type DynamoDBEvent struct {
	Records []DynamoDBEventRecord
}

// DynamoDBEventRecord mirrors events.DynamoDBEventRecord type, removing unused
// fields.
type DynamoDBEventRecord struct {
	Change         DynamoDBStreamRecord `json:"dynamodb"`
	EventID        string
	EventName      string
	EventVersion   string
	EventSourceArn string
}

// DynamoDBStreamRecord mirrors events.DynamoDBStreamRecord type, removing
// unused fields.
type DynamoDBStreamRecord struct {
	ApproximateCreationDateTime events.SecondsEpochTime
	SizeBytes                   int64
	StreamViewType              string
}

// KinesisEvent mirrors events.KinesisEvent type, removing unused fields.
type KinesisEvent struct {
	Records []KinesisEventRecord
}

// KinesisEventRecord mirrors events.KinesisEventRecord type, removing unused
// fields.
type KinesisEventRecord struct {
	EventID        string
	EventName      string
	EventSourceArn string
	EventVersion   string
	Kinesis        KinesisRecord
}

// KinesisRecord mirrors events.KinesisRecord type, removing unused fields.
type KinesisRecord struct {
	ApproximateArrivalTimestamp events.SecondsEpochTime
	PartitionKey                string
}

// EventBridgeEvent is used for unmarshalling a EventBridge event.  AWS Go
// libraries do not provide this type of event for deserialization.
type EventBridgeEvent struct {
	DetailType string `json:"detail-type"`
	Source     string
	StartTime  string
}

// S3Event mirrors events.S3Event type, removing unused fields.
type S3Event struct {
	Records []S3EventRecord
}

// S3EventRecord mirrors events.S3EventRecord type, removing unused fields.
type S3EventRecord struct {
	EventSource string
	EventTime   time.Time
	EventName   string
	S3          S3Entity
}

// S3Entity mirrors events.S3Entity type, removing unused fields.
type S3Entity struct {
	Bucket S3Bucket
	Object S3Object
}

// S3Bucket mirrors events.S3Bucket type, removing unused fields.
type S3Bucket struct {
	Name string
	Arn  string
}

// S3Object mirrors events.S3Object type, removing unused fields.
type S3Object struct {
	Key  string
	Size int64
	ETag string
}

// SNSEvent mirrors events.SNSEvent type, removing unused fields.
type SNSEvent struct {
	Records []SNSEventRecord
}

// SNSEventRecord mirrors events.SNSEventRecord type, removing unused fields.
type SNSEventRecord struct {
	SNS SNSEntity
}

// SNSEntity mirrors events.SNSEntity type, removing unused fields.
type SNSEntity struct {
	MessageID         string
	Type              string
	TopicArn          string
	MessageAttributes map[string]interface{}
	Timestamp         time.Time
	Subject           string
}

// SQSEvent mirrors events.SQSEvent type, removing unused fields.
type SQSEvent struct {
	Records []SQSMessage
}

// SQSMessage mirrors events.SQSMessage type, removing unused fields.
type SQSMessage struct {
	ReceiptHandle     string
	Body              string
	Attributes        map[string]string
	MessageAttributes map[string]SQSMessageAttribute
	EventSourceARN    string
}

// SQSMessageAttribute mirrors events.SQSMessageAttribute type, removing unused
// fields.
type SQSMessageAttribute struct {
	StringValue *string
	BinaryValue []byte
	DataType    string
}

// LambdaFunctionURLRequest mirrors events.LambdaFunctionURLRequest type,
// removing unused fields.
type LambdaFunctionURLRequest struct {
	Headers        map[string]string
	RequestContext LambdaFunctionURLRequestContext
}

// LambdaFunctionURLRequestContext mirrors
// events.LambdaFunctionURLRequestContext type, removing unused fields.
type LambdaFunctionURLRequestContext struct {
	RequestID  string
	APIID      string
	DomainName string
	TimeEpoch  int64
	HTTP       LambdaFunctionURLRequestContextHTTPDescription
}

// LambdaFunctionURLRequestContextHTTPDescription mirrors
// events.LambdaFunctionURLRequestContextHTTPDescription type, removing unused
// fields.
type LambdaFunctionURLRequestContextHTTPDescription struct {
	Method    string
	Path      string
	Protocol  string
	SourceIP  string
	UserAgent string
}
