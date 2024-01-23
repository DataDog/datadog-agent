// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package trigger

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
)

// GetAWSPartitionByRegion parses an AWS region and returns an AWS partition
func GetAWSPartitionByRegion(region string) string {
	if strings.HasPrefix(region, "us-gov-") {
		return "aws-us-gov"
	} else if strings.HasPrefix(region, "cn-") {
		return "aws-cn"
	} else {
		return "aws"
	}
}

// ExtractAPIGatewayEventARN returns an ARN from an APIGatewayProxyRequest
func ExtractAPIGatewayEventARN(event events.APIGatewayProxyRequest, region string) string {
	panic("not called")
}

// ExtractAPIGatewayV2EventARN returns an ARN from an APIGatewayV2HTTPRequest
func ExtractAPIGatewayV2EventARN(event events.APIGatewayV2HTTPRequest, region string) string {
	panic("not called")
}

// ExtractAPIGatewayWebSocketEventARN returns an ARN from an APIGatewayWebsocketProxyRequest
func ExtractAPIGatewayWebSocketEventARN(event events.APIGatewayWebsocketProxyRequest, region string) string {
	panic("not called")
}

// ExtractAPIGatewayCustomAuthorizerEventARN returns an ARN from an APIGatewayCustomAuthorizerRequest
func ExtractAPIGatewayCustomAuthorizerEventARN(event events.APIGatewayCustomAuthorizerRequest) string {
	panic("not called")
}

// ExtractAPIGatewayCustomAuthorizerRequestTypeEventARN returns an ARN from an APIGatewayCustomAuthorizerRequestTypeRequest
func ExtractAPIGatewayCustomAuthorizerRequestTypeEventARN(event events.APIGatewayCustomAuthorizerRequestTypeRequest) string {
	panic("not called")
}

// ExtractAlbEventARN returns an ARN from an ALBTargetGroupRequest
func ExtractAlbEventARN(event events.ALBTargetGroupRequest) string {
	panic("not called")
}

// ExtractCloudwatchEventARN returns an ARN from a CloudWatchEvent
func ExtractCloudwatchEventARN(event events.CloudWatchEvent) string {
	panic("not called")
}

// ExtractCloudwatchLogsEventARN returns an ARN from a CloudwatchLogsEvent
func ExtractCloudwatchLogsEventARN(event events.CloudwatchLogsEvent, region string, accountID string) (string, error) {
	panic("not called")
}

// ExtractDynamoDBStreamEventARN returns an ARN from a DynamoDBEvent
func ExtractDynamoDBStreamEventARN(event events.DynamoDBEvent) string {
	panic("not called")
}

// ExtractKinesisStreamEventARN returns an ARN from a KinesisEvent
func ExtractKinesisStreamEventARN(event events.KinesisEvent) string {
	panic("not called")
}

// ExtractS3EventArn returns an ARN from a S3Event
func ExtractS3EventArn(event events.S3Event) string {
	panic("not called")
}

// ExtractSNSEventArn returns an ARN from a SNSEvent
func ExtractSNSEventArn(event events.SNSEvent) string {
	panic("not called")
}

// ExtractSQSEventARN returns an ARN from a SQSEvent
func ExtractSQSEventARN(event events.SQSEvent) string {
	panic("not called")
}

// GetTagsFromAPIGatewayEvent returns a tagset containing http tags from an
// APIGatewayProxyRequest
func GetTagsFromAPIGatewayEvent(event events.APIGatewayProxyRequest) map[string]string {
	panic("not called")
}

// GetTagsFromAPIGatewayV2HTTPRequest returns a tagset containing http tags from an
// APIGatewayProxyRequest
func GetTagsFromAPIGatewayV2HTTPRequest(event events.APIGatewayV2HTTPRequest) map[string]string {
	panic("not called")
}

// GetTagsFromAPIGatewayCustomAuthorizerEvent returns a tagset containing http tags from an
// APIGatewayCustomAuthorizerRequest
func GetTagsFromAPIGatewayCustomAuthorizerEvent(event events.APIGatewayCustomAuthorizerRequest) map[string]string {
	panic("not called")
}

// GetTagsFromAPIGatewayCustomAuthorizerRequestTypeEvent returns a tagset containing http tags from an
// APIGatewayCustomAuthorizerRequestTypeRequest
func GetTagsFromAPIGatewayCustomAuthorizerRequestTypeEvent(event events.APIGatewayCustomAuthorizerRequestTypeRequest) map[string]string {
	panic("not called")
}

// GetTagsFromALBTargetGroupRequest returns a tagset containing http tags from an
// ALBTargetGroupRequest
func GetTagsFromALBTargetGroupRequest(event events.ALBTargetGroupRequest) map[string]string {
	panic("not called")
}

// GetTagsFromLambdaFunctionURLRequest returns a tagset containing http tags from a
// LambdaFunctionURLRequest
func GetTagsFromLambdaFunctionURLRequest(event events.LambdaFunctionURLRequest) map[string]string {
	panic("not called")
}

// GetStatusCodeFromHTTPResponse parses a generic payload and returns
// a status code, if it contains one. Returns an empty string if it does not,
// or an error in case of json parsing error.
func GetStatusCodeFromHTTPResponse(rawPayload []byte) (string, error) {
	panic("not called")
}

// ParseArn parses an AWS ARN and returns the region and account
func ParseArn(arn string) (string, string, string, error) {
	panic("not called")
}
