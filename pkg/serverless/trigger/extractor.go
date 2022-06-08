package trigger

import (
	"fmt"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

func isHTTPTriggerEvent(eventSource AWSEventType) bool {
	return eventSource == APIGatewayEvent ||
		eventSource == APIGatewayV2Event ||
		eventSource == APIGatewayWebsocketEvent ||
		eventSource == ALBEvent ||
		eventSource == LambdaFunctionURLEvent
}

func getAWSPartitionByRegion(region string) string {
	if strings.HasPrefix(region, "us-gov-") {
		return "aws-us-gov"
	} else if strings.HasPrefix(region, "cn-") {
		return "aws-cn"
	} else {
		return "aws"
	}
}

func ExtractAPIGatewayEventARN(event events.APIGatewayProxyRequest, region string) string {
	requestContext := event.RequestContext
	return fmt.Sprintf("arn:%v:apigateway:%v::/restapis/%v/stages/%v", getAWSPartitionByRegion(region), region, requestContext.APIID, requestContext.Stage)
}

func ExtractAPIGatewayV2EventARN(event events.APIGatewayV2HTTPRequest, region string) string {
	requestContext := event.RequestContext
	return fmt.Sprintf("arn:%v:apigateway:%v::/restapis/%v/stages/%v", getAWSPartitionByRegion(region), region, requestContext.APIID, requestContext.Stage)
}

func ExtractAPIGatewayWebSocketEventARN(event events.APIGatewayWebsocketProxyRequest, region string) string {
	requestContext := event.RequestContext
	return fmt.Sprintf("arn:%v:apigateway:%v::/restapis/%v/stages/%v", getAWSPartitionByRegion(region), region, requestContext.APIID, requestContext.Stage)
}

func ExtractAlbEventARN(event events.ALBTargetGroupRequest) string {
	return event.RequestContext.ELB.TargetGroupArn
}

func ExtractCloudwatchEventARN(event events.CloudWatchEvent) string {
	return event.Resources[0]
}

func ExtractCloudwatchLogsEventARN(event events.CloudwatchLogsEvent, region string, accountID string) (string, error) {
	decodedLog, err := event.AWSLogs.Parse()
	if err != nil {
		return "", fmt.Errorf("Couldn't decode Cloudwatch Logs event: %v", err)
	}
	return fmt.Sprintf("arn:%v:logs:%v:%v:log-group:%v", getAWSPartitionByRegion(region), region, accountID, decodedLog.LogGroup), nil
}

func ExtractDynamoDBStreamEventARN(event events.DynamoDBEvent) string {
	return event.Records[0].EventSourceArn
}

func ExtractKinesisStreamEventARN(event events.KinesisEvent) string {
	return event.Records[0].EventSourceArn
}

func ExtractS3EventArn(event events.S3Event) string {
	return event.Records[0].EventSource
}

func ExtractSNSEventArn(event events.SNSEvent) string {
	return event.Records[0].SNS.TopicArn
}

func ExtractSQSEventARN(event events.SQSEvent) string {
	return event.Records[0].EventSourceARN
}

func GetTagsFromAPIGatewayEvent(event events.APIGatewayProxyRequest) map[string]string {
	httpTags := make(map[string]string)
	if event.RequestContext.DomainName != "" {
		httpTags["http.url"] = event.RequestContext.DomainName
	}
	httpTags["http.url_details.path"] = event.RequestContext.Path
	httpTags["http.method"] = event.RequestContext.HTTPMethod
	if event.Headers != nil {
		if event.Headers["Referer"] != "" {
			httpTags["http.referer"] = event.Headers["Referer"]
		}
	}
	return httpTags
}

func GetTagsFromAPIGatewayV2HTTPRequest(event events.APIGatewayV2HTTPRequest) map[string]string {
	httpTags := make(map[string]string)
	httpTags["http.url"] = event.RequestContext.DomainName
	httpTags["http.url_details.path"] = event.RequestContext.HTTP.Path
	httpTags["http.method"] = event.RequestContext.HTTP.Method
	if event.Headers != nil {
		if event.Headers["Referer"] != "" {
			httpTags["http.referer"] = event.Headers["Referer"]
		}
	}
	return httpTags
}

func GetTagsFromALBTargetGroupRequest(event events.ALBTargetGroupRequest) map[string]string {
	httpTags := make(map[string]string)
	httpTags["http.url_details.path"] = event.Path
	httpTags["http.method"] = event.HTTPMethod
	if event.Headers != nil {
		if event.Headers["Referer"] != "" {
			httpTags["http.referer"] = event.Headers["Referer"]
		}
	}
	return httpTags
}

func GetTagsFromLambdaFunctionURLRequest(event events.LambdaFunctionURLRequest) map[string]string {
	httpTags := make(map[string]string)
	if event.RequestContext.DomainName != "" {
		httpTags["http.url"] = event.RequestContext.DomainName
	}
	httpTags["http.url_details.path"] = event.RequestContext.HTTP.Path
	httpTags["http.method"] = event.RequestContext.HTTP.Method
	if event.Headers != nil {
		if event.Headers["Referer"] != "" {
			httpTags["http.referer"] = event.Headers["Referer"]
		}
	}
	return httpTags
}

// TODO: Support for Appsync? Not in the JS library

// TODO: Where is Cloudfront?
// This looks like it's not in aws-lambda-go because this is a lambda@edge function, which can't run
// extensions anwyays?
// func extractCloudFrontRequestEventARN(event events.CloudFrontRequestEvent) string {
// 	return
// }

// TODO: Where is SNSSQSEvent?
// Is this a special case, or is it just an SQS message wrapped in a
// SNS event/SNS message wrapped in a SQS event?

// TODO: Where is EventBridge?
// It looks like EventBridge is an OpenAPI spec with a few different schemas? Not
// sure how to handle this.
// https://github.com/aws/aws-lambda-go/issues/51#issuecomment-662256535
