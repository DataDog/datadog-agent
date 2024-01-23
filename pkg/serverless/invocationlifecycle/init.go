// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package invocationlifecycle

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
)

const (
	tagFunctionTriggerEventSource    = "function_trigger.event_source"
	tagFunctionTriggerEventSourceArn = "function_trigger.event_source_arn"
)

func (lp *LifecycleProcessor) initFromAPIGatewayEvent(event events.APIGatewayProxyRequest, region string) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromAPIGatewayV2Event(event events.APIGatewayV2HTTPRequest, region string) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromAPIGatewayWebsocketEvent(event events.APIGatewayWebsocketProxyRequest, region string) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromAPIGatewayLambdaAuthorizerTokenEvent(event events.APIGatewayCustomAuthorizerRequest) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromAPIGatewayLambdaAuthorizerRequestParametersEvent(event events.APIGatewayCustomAuthorizerRequestTypeRequest) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromALBEvent(event events.ALBTargetGroupRequest) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromCloudWatchEvent(event events.CloudWatchEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromCloudWatchLogsEvent(event events.CloudwatchLogsEvent, region string, accountID string) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromDynamoDBStreamEvent(event events.DynamoDBEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromEventBridgeEvent(event events.EventBridgeEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromKinesisStreamEvent(event events.KinesisEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromS3Event(event events.S3Event) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromSNSEvent(event events.SNSEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromSQSEvent(event events.SQSEvent) {
	panic("not called")
}

func (lp *LifecycleProcessor) initFromLambdaFunctionURLEvent(event events.LambdaFunctionURLRequest, region string, accountID string, functionName string) {
	panic("not called")
}
