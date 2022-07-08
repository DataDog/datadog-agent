// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, "arn:aws:sns:sa-east-1:601427279990:serverlessTracingTopicPy", span.Meta[topicARN])
	assert.Equal(t, "serverlessTracingTopicPy", span.Meta[topicName])
	assert.Equal(t, "Notification", span.Meta[metadataType])
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
