package invocationlifecycle

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-lambda-go/events"
)

func (lp *LifecycleProcessor) initFromAPIGatewayEvent(event events.APIGatewayProxyRequest, region string) {
	if !lp.DetectLambdaLibrary() && lp.InferredSpansEnabled {
		lp.GetInferredSpan().EnrichInferredSpanWithAPIGatewayRESTEvent(event)
	}

	lp.addTag("function_trigger.event_source", "api-gateway")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractAPIGatewayEventARN(event, region))
	lp.addTags(trigger.GetTagsFromAPIGatewayEvent(event))
}

func (lp *LifecycleProcessor) initFromAPIGatewayV2Event(event events.APIGatewayV2HTTPRequest, region string) {
	if !lp.DetectLambdaLibrary() && lp.InferredSpansEnabled {
		lp.GetInferredSpan().EnrichInferredSpanWithAPIGatewayHTTPEvent(event)
	}

	lp.addTag("function_trigger.event_source", "api-gateway")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractAPIGatewayV2EventARN(event, region))
	lp.addTags(trigger.GetTagsFromAPIGatewayV2HTTPRequest(event))
}

func (lp *LifecycleProcessor) initFromAPIGatewayWebsocketEvent(event events.APIGatewayWebsocketProxyRequest, region string) {
	if !lp.DetectLambdaLibrary() && lp.InferredSpansEnabled {
		lp.GetInferredSpan().EnrichInferredSpanWithAPIGatewayWebsocketEvent(event)
	}

	lp.addTag("function_trigger.event_source", "api-gateway")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractAPIGatewayWebSocketEventARN(event, region))
}

func (lp *LifecycleProcessor) initFromALBEvent(event events.ALBTargetGroupRequest) {
	lp.addTag("function_trigger.event_source", "application-load-balancer")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractAlbEventARN(event))
	lp.addTags(trigger.GetTagsFromALBTargetGroupRequest(event))
}

func (lp *LifecycleProcessor) initFromCloudWatchEvent(event events.CloudWatchEvent) {
	lp.addTag("function_trigger.event_source", "cloudwatch-events")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractCloudwatchEventARN(event))
}

func (lp *LifecycleProcessor) initFromCloudWatchLogsEvent(event events.CloudwatchLogsEvent, region string, accountID string) {
	arn, err := trigger.ExtractCloudwatchLogsEventARN(event, region, accountID)
	if err != nil {
		log.Debugf("Error parsing event ARN from cloudwatch logs event: %v", err)
		return
	}
	lp.addTag("function_trigger.event_source", "cloudwatch-logs")
	lp.addTag("function_trigger.event_source_arn", arn)
}

func (lp *LifecycleProcessor) initFromDynamoDBStreamEvent(event events.DynamoDBEvent) {
	lp.addTag("function_trigger.event_source", "dynamodb")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractDynamoDBStreamEventARN(event))
}

func (lp *LifecycleProcessor) initFromKinesisStreamEvent(event events.KinesisEvent) {
	lp.addTag("function_trigger.event_source", "kinesis")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractKinesisStreamEventARN(event))
}

func (lp *LifecycleProcessor) initFromS3Event(event events.S3Event) {
	lp.addTag("function_trigger.event_source", "s3")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractS3EventArn(event))
}

func (lp *LifecycleProcessor) initFromSNSEvent(event events.SNSEvent) {
	if !lp.DetectLambdaLibrary() && lp.InferredSpansEnabled {
		lp.GetInferredSpan().EnrichInferredSpanWithSNSEvent(event)
	}
	lp.addTag("function_trigger.event_source", "sns")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractSNSEventArn(event))
}

func (lp *LifecycleProcessor) initFromSQSEvent(event events.SQSEvent) {
	lp.addTag("function_trigger.event_source", "sqs")
	lp.addTag("function_trigger.event_source_arn", trigger.ExtractSQSEventARN(event))

}

func (lp *LifecycleProcessor) initFromLambdaFunctionURLEvent(event events.LambdaFunctionURLRequest) {
	lp.addTag("function_trigger.event_source", "lambda-function-url")
}
