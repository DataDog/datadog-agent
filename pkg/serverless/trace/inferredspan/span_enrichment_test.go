// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	dataFile = "../testdata/event_samples/"
)

func TestEnrichInferredSpanWithAPIGatewayRESTEvent(t *testing.T) {
	var apiGatewayRestEvent events.APIGatewayProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway.json"), &apiGatewayRestEvent)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent)

	span := inferredSpan.Span

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1428582896000000000), span.Start)
	assert.Equal(t, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com", span.Service)
	assert.Equal(t, "aws.apigateway", span.Name)
	assert.Equal(t, "POST /path/to/resource", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "1234567890", span.Meta[apiID])
	assert.Equal(t, "1234567890", span.Meta[apiName])
	assert.Equal(t, "/path/to/resource", span.Meta[endpoint])
	assert.Equal(t, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com/path/to/resource", span.Meta[httpURL])
	assert.Equal(t, "aws.apigateway.rest", span.Meta[operationName])
	assert.Equal(t, "c6af9ac6-7b61-11e6-9a41-93e8deadbeef", span.Meta[requestID])
	assert.Equal(t, "POST /path/to/resource", span.Meta[resourceNames])
	assert.Equal(t, "prod", span.Meta[stage])
	assert.False(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithAPIGatewayNonProxyAsyncRESTEvent(t *testing.T) {
	var apiGatewayRestEvent events.APIGatewayProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway-non-proxy-async.json"), &apiGatewayRestEvent)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent)

	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631210915251000000), span.Start)
	assert.Equal(t, "lgxbo6a518.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.apigateway", span.Name)
	assert.Equal(t, "GET /http/get", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "lgxbo6a518", span.Meta[apiID])
	assert.Equal(t, "lgxbo6a518", span.Meta[apiName])
	assert.Equal(t, "/http/get", span.Meta[endpoint])
	assert.Equal(t, "lgxbo6a518.execute-api.sa-east-1.amazonaws.com/http/get", span.Meta[httpURL])
	assert.Equal(t, "aws.apigateway.rest", span.Meta[operationName])
	assert.Equal(t, "7bf3b161-f698-432c-a639-6fef8b445137", span.Meta[requestID])
	assert.Equal(t, "GET /http/get", span.Meta[resourceNames])
	assert.Equal(t, "dev", span.Meta[stage])
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithAPIGatewayHTTPEvent(t *testing.T) {
	var apiGatewayHTTPEvent events.APIGatewayV2HTTPRequest
	_ = json.Unmarshal(getEventFromFile("http-api.json"), &apiGatewayHTTPEvent)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayHTTPEvent(apiGatewayHTTPEvent)

	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631212283738000000), span.Start)
	assert.Equal(t, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.httpapi", span.Name)
	assert.Equal(t, "GET /httpapi/get", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "GET", span.Meta[httpMethod])
	assert.Equal(t, "HTTP/1.1", span.Meta[httpProtocol])
	assert.Equal(t, "38.122.226.210", span.Meta[httpSourceIP])
	assert.Equal(t, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com/httpapi/get", span.Meta[httpURL])
	assert.Equal(t, "curl/7.64.1", span.Meta[httpUserAgent])
	assert.Equal(t, "aws.httpapi", span.Meta[operationName])
	assert.Equal(t, "FaHnXjKCGjQEJ7A=", span.Meta[requestID])
	assert.Equal(t, "GET /httpapi/get", span.Meta[resourceNames])
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketDefaultEvent(t *testing.T) {
	var apiGatewayWebsocketEvent events.APIGatewayWebsocketProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-default.json"), &apiGatewayWebsocketEvent)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	inferredSpan.EnrichInferredSpanWithAPIGatewayWebsocketEvent(apiGatewayWebsocketEvent)

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631285061365000000), span.Start)
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.apigateway.websocket", span.Name)
	assert.Equal(t, "$default", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "p62c47itsb", span.Meta[apiID])
	assert.Equal(t, "p62c47itsb", span.Meta[apiName])
	assert.Equal(t, "Fc5SzcoYGjQCJlg=", span.Meta[connectionID])
	assert.Equal(t, "$default", span.Meta[endpoint])
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com$default", span.Meta[httpURL])
	assert.Equal(t, "IN", span.Meta[messageDirection])
	assert.Equal(t, "aws.apigateway.websocket", span.Meta[operationName])
	assert.Equal(t, "Fc5S3EvdGjQFtsQ=", span.Meta[requestID])
	assert.Equal(t, "$default", span.Meta[resourceNames])
	assert.Equal(t, "dev", span.Meta[stage])
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketConnectEvent(t *testing.T) {
	var apiGatewayWebsocketEvent events.APIGatewayWebsocketProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-connect.json"), &apiGatewayWebsocketEvent)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	inferredSpan.EnrichInferredSpanWithAPIGatewayWebsocketEvent(apiGatewayWebsocketEvent)

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631284003071000000), span.Start)
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.apigateway.websocket", span.Name)
	assert.Equal(t, "$connect", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "p62c47itsb", span.Meta[apiID])
	assert.Equal(t, "p62c47itsb", span.Meta[apiName])
	assert.Equal(t, "Fc2tgfl3mjQCJfA=", span.Meta[connectionID])
	assert.Equal(t, "$connect", span.Meta[endpoint])
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com$connect", span.Meta[httpURL])
	assert.Equal(t, "IN", span.Meta[messageDirection])
	assert.Equal(t, "aws.apigateway.websocket", span.Meta[operationName])
	assert.Equal(t, "Fc2tgH1RmjQFnOg=", span.Meta[requestID])
	assert.Equal(t, "$connect", span.Meta[resourceNames])
	assert.Equal(t, "dev", span.Meta[stage])
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketDisconnectEvent(t *testing.T) {
	var apiGatewayWebsocketEvent events.APIGatewayWebsocketProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-disconnect.json"), &apiGatewayWebsocketEvent)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	inferredSpan.EnrichInferredSpanWithAPIGatewayWebsocketEvent(apiGatewayWebsocketEvent)

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631284034737000000), span.Start)
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.apigateway.websocket", span.Name)
	assert.Equal(t, "$disconnect", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "p62c47itsb", span.Meta[apiID])
	assert.Equal(t, "p62c47itsb", span.Meta[apiName])
	assert.Equal(t, "Fc2tgfl3mjQCJfA=", span.Meta[connectionID])
	assert.Equal(t, "$disconnect", span.Meta[endpoint])
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com$disconnect", span.Meta[httpURL])
	assert.Equal(t, "IN", span.Meta[messageDirection])
	assert.Equal(t, "aws.apigateway.websocket", span.Meta[operationName])
	assert.Equal(t, "Fc2ydE4LmjQFhdg=", span.Meta[requestID])
	assert.Equal(t, "$disconnect", span.Meta[resourceNames])
	assert.Equal(t, "dev", span.Meta[stage])
}

func TestEnrichInferredSpanWithSNSEvent(t *testing.T) {
	var snsRequest events.SNSEvent
	_ = json.Unmarshal(getEventFromFile("sns.json"), &snsRequest)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSNSEvent(snsRequest)

	span := inferredSpan.Span

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, formatISOStartTime("2022-01-31T14:13:41.637Z"), span.Start)
	assert.Equal(t, "sns", span.Service)
	assert.Equal(t, "aws.sns", span.Name)
	assert.Equal(t, "serverlessTracingTopicPy", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "87056a47-f506-5d77-908b-303605d3b197", span.Meta[messageID])
	assert.Equal(t, "aws.sns", span.Meta[operationName])
	assert.Equal(t, "serverlessTracingTopicPy", span.Meta[resourceNames])
	assert.Equal(t, "Hello", span.Meta[subject])
	assert.Equal(t, "arn:aws:sns:sa-east-1:425362996713:serverlessTracingTopicPy", span.Meta[topicARN])
	assert.Equal(t, "serverlessTracingTopicPy", span.Meta[topicName])
	assert.Equal(t, "Notification", span.Meta[metadataType])
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanForS3Event(t *testing.T) {
	var s3Request events.S3Event
	_ = json.Unmarshal(getEventFromFile("s3.json"), &s3Request)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithS3Event(s3Request)

	span := inferredSpan.Span

	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, formatISOStartTime("1970-01-01T00:00:00.000Z"), span.Start)
	assert.Equal(t, "s3", span.Service)
	assert.Equal(t, "aws.s3", span.Name)
	assert.Equal(t, "example-bucket", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.s3", span.Meta[operationName])
	assert.Equal(t, "example-bucket", span.Meta[resourceNames])
	assert.Equal(t, "ObjectCreated:Put", span.Meta[eventName])
	assert.Equal(t, "example-bucket", span.Meta[bucketName])
	assert.Equal(t, "arn:aws:s3:::example-bucket", span.Meta[bucketARN])
	assert.Equal(t, "test/key", span.Meta[objectKey])
	assert.Equal(t, "1024", span.Meta[objectSize])
	assert.Equal(t, "0123456789abcdef0123456789abcdef", span.Meta[objectETag])
}
func TestEnrichInferredSpanWithEventBridgeEvent(t *testing.T) {
	var eventBridgeEvent EventBridgeEvent
	_ = json.Unmarshal(getEventFromFile("eventbridge-custom.json"), &eventBridgeEvent)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithEventBridgeEvent(eventBridgeEvent)
	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, formatISOStartTime("2017-12-22T18:43:48Z"), span.Start)
	assert.Equal(t, "eventbridge", span.Service)
	assert.Equal(t, "aws.eventbridge", span.Name)
	assert.Equal(t, "eventbridge.custom.event.sender", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.eventbridge", span.Meta[operationName])
	assert.Equal(t, "eventbridge.custom.event.sender", span.Meta[resourceNames])
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithSQSEvent(t *testing.T) {
	var sqsRequest events.SQSEvent
	_ = json.Unmarshal(getEventFromFile("sqs.json"), &sqsRequest)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSQSEvent(sqsRequest)
	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1634662094538000000), span.Start)
	assert.Equal(t, "sqs", span.Service)
	assert.Equal(t, "aws.sqs", span.Name)
	assert.Equal(t, "InferredSpansQueueNode", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.sqs", span.Meta[operationName])
	assert.Equal(t, "InferredSpansQueueNode", span.Meta[resourceNames])
	assert.Equal(t, "InferredSpansQueueNode", span.Meta[queueName])
	assert.Equal(t, "arn:aws:sqs:sa-east-1:425362996713:InferredSpansQueueNode", span.Meta[eventSourceArn])
	assert.Equal(t, "AQEBnxFcyzQZhkrLV/TrSpn0VBszuq4a5/u66uyGRdUKuvXMurd6RRV952L+arORbE4MlGqWLUxurzYH9mKvc/A3MYjmGwQvvhp6uK5c7gXxg6tvHVAlsEFmTB0p35dxfGCmtrJbzdPjVtmcucPEpRx7z51tQokgGWuJbqx3Z9MVRD+6dyO3o6Zu6G3oWUgiUZ0dxhNoIIeT6xr/tEsoWhGK9ZUPRJ7e0BM/UZKfkecX1CVgVZ8J/t8fHRklJd34S6pN99SPNBKx+1lOZCelm2MihbQR6zax8bkhwL3glxYP83MxexvfOELA3G/6jx96oQ4mQdJASsKFUzvcs2NUxX+0bBVX9toS7MW/Udv+3CiQwSjjkc18A385QHtNrJDRbH33OUxFCqN5CcUMiGvEFed5EQ==", span.Meta[receiptHandle])
	assert.Equal(t, "AROAYYB64AB3LSVUYFP5T:harv-inferred-spans-dev-initSender", span.Meta[senderID])
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithKinesisEvent(t *testing.T) {
	var kinesisRequest events.KinesisEvent
	_ = json.Unmarshal(getEventFromFile("kinesis.json"), &kinesisRequest)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithKinesisEvent(kinesisRequest)
	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1643638425163000106), span.Start)
	assert.Equal(t, "kinesis", span.Service)
	assert.Equal(t, "aws.kinesis", span.Name)
	assert.Equal(t, "stream/kinesisStream", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.kinesis", span.Meta[operationName])
	assert.Equal(t, "stream/kinesisStream", span.Meta[resourceNames])
	assert.Equal(t, "stream/kinesisStream", span.Meta[streamName])
	assert.Equal(t, "shardId-000000000002", span.Meta[shardID])
	assert.Equal(t, "arn:aws:kinesis:sa-east-1:425362996713:stream/kinesisStream", span.Meta[eventSourceArn])
	assert.Equal(t, "shardId-000000000002:49624230154685806402418173680709770494154422022871973922", span.Meta[eventID])
	assert.Equal(t, "aws:kinesis:record", span.Meta[eventName])
	assert.Equal(t, "1.0", span.Meta[eventVersion])
	assert.Equal(t, "partitionkey", span.Meta[partitionKey])
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithDynamoDBEvent(t *testing.T) {
	var dynamoRequest events.DynamoDBEvent
	_ = json.Unmarshal(getEventFromFile("dynamodb.json"), &dynamoRequest)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithDynamoDBEvent(dynamoRequest)
	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, time.Unix(1428537600, 0).UnixNano(), span.Start)
	assert.Equal(t, "dynamodb", span.Service)
	assert.Equal(t, "aws.dynamodb", span.Name)
	assert.Equal(t, "ExampleTableWithStream", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.dynamodb", span.Meta[operationName])
	assert.Equal(t, "ExampleTableWithStream", span.Meta[resourceNames])
	assert.Equal(t, "ExampleTableWithStream", span.Meta[tableName])
	assert.Equal(t, "arn:aws:dynamodb:us-east-1:123456789012:table/ExampleTableWithStream/stream/2015-06-27T00:48:05.899", span.Meta[eventSourceArn])
	assert.Equal(t, "c4ca4238a0b923820dcc509a6f75849b", span.Meta[eventID])
	assert.Equal(t, "INSERT", span.Meta[eventName])
	assert.Equal(t, "1.1", span.Meta[eventVersion])
	assert.Equal(t, "NEW_AND_OLD_IMAGES", span.Meta[streamViewType])
	assert.Equal(t, "26", span.Meta[sizeBytes])
	assert.True(t, inferredSpan.IsAsync)
}

func TestFormatISOStartTime(t *testing.T) {
	isotime := "2022-01-31T14:13:41.637Z"
	startTime := formatISOStartTime(isotime)
	assert.Equal(t, int64(1643638421637000000), startTime)

}

func TestFormatInvalidISOStartTime(t *testing.T) {
	isotime := "invalid"
	startTime := formatISOStartTime(isotime)
	assert.Equal(t, int64(0), startTime)
}

func getEventFromFile(filename string) []byte {
	event, _ := os.ReadFile(dataFile + filename)
	return event
}

func mockInferredSpan() InferredSpan {
	var inferredSpan InferredSpan
	inferredSpan.Span = &pb.Span{}
	inferredSpan.Span.TraceID = uint64(7353030974370088224)
	inferredSpan.Span.SpanID = uint64(8048964810003407541)
	return inferredSpan
}

func TestCalculateStartTime(t *testing.T) {
	assert.Equal(t, int64(1651863561696000000), calculateStartTime(1651863561696))
}
