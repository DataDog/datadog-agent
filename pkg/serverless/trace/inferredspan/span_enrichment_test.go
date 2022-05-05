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

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

const (
	dataFile = "../testdata/event_samples/"
)

func TestSetSynchronicityFalse(t *testing.T) {
	var attributes EventKeys
	attributes.Headers.InvocationType = ""
	span := GenerateInferredSpan(time.Now())
	span.IsAsync = isAsyncEvent(attributes)

	assert.False(t, span.IsAsync)
}

func TestSetSynchronicityTrue(t *testing.T) {
	var attributes EventKeys
	attributes.Headers.InvocationType = "Event"
	span := GenerateInferredSpan(time.Now())
	span.IsAsync = isAsyncEvent(attributes)

	assert.True(t, span.IsAsync)
}

func TestEnrichInferredSpanWithAPIGatewayRESTEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("api-gateway.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	inferredSpan.IsAsync = isAsyncEvent(eventKeys)
	EnrichInferredSpanWithAPIGatewayRESTEvent(eventKeys, inferredSpan)

	span := inferredSpan.Span

	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1428582896000000000))
	assert.Equal(t, span.Service, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com")
	assert.Equal(t, span.Name, "aws.apigateway")
	assert.Equal(t, span.Resource, "POST /path/to/resource")
	assert.Equal(t, span.Type, "http")
	assert.Equal(t, span.Meta[APIID], "1234567890")
	assert.Equal(t, span.Meta[APIName], "1234567890")
	assert.Equal(t, span.Meta[Endpoint], "/path/to/resource")
	assert.Equal(t, span.Meta[HTTPURL], "70ixmpl4fl.execute-api.us-east-2.amazonaws.com/path/to/resource")
	assert.Equal(t, span.Meta[OperationName], "aws.apigateway.rest")
	assert.Equal(t, span.Meta[RequestID], "c6af9ac6-7b61-11e6-9a41-93e8deadbeef")
	assert.Equal(t, span.Meta[ResourceNames], "POST /path/to/resource")
	assert.Equal(t, span.Meta[Stage], "prod")
	assert.False(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithAPIGatewayNonProxyAsyncRESTEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("api-gateway-non-proxy-async.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	inferredSpan.IsAsync = isAsyncEvent(eventKeys)
	EnrichInferredSpanWithAPIGatewayRESTEvent(eventKeys, inferredSpan)

	span := inferredSpan.Span
	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1631210915000000000))
	assert.Equal(t, span.Service, "lgxbo6a518.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Name, "aws.apigateway")
	assert.Equal(t, span.Resource, "GET /http/get")
	assert.Equal(t, span.Type, "http")
	assert.Equal(t, span.Meta[APIID], "lgxbo6a518")
	assert.Equal(t, span.Meta[APIName], "lgxbo6a518")
	assert.Equal(t, span.Meta[Endpoint], "/http/get")
	assert.Equal(t, span.Meta[HTTPURL], "lgxbo6a518.execute-api.sa-east-1.amazonaws.com/http/get")
	assert.Equal(t, span.Meta[OperationName], "aws.apigateway.rest")
	assert.Equal(t, span.Meta[RequestID], "7bf3b161-f698-432c-a639-6fef8b445137")
	assert.Equal(t, span.Meta[ResourceNames], "GET /http/get")
	assert.Equal(t, span.Meta[Stage], "dev")
	assert.True(t, inferredSpan.IsAsync)
}

func TestEnrichInferredSpanWithAPIGatewayHTTPEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("http-api.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	EnrichInferredSpanWithAPIGatewayHTTPEvent(eventKeys, inferredSpan)

	span := inferredSpan.Span
	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1631212283000000000))
	assert.Equal(t, span.Service, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Name, "aws.httpapi")
	assert.Equal(t, span.Resource, "GET ")
	assert.Equal(t, span.Type, "http")
	assert.Equal(t, span.Meta[HTTPMethod], "GET")
	assert.Equal(t, span.Meta[HTTPProtocol], "HTTP/1.1")
	assert.Equal(t, span.Meta[HTTPSourceIP], "38.122.226.210")
	assert.Equal(t, span.Meta[HTTPURL], "x02yirxc7a.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Meta[HTTPUserAgent], "curl/7.64.1")
	assert.Equal(t, span.Meta[OperationName], "aws.httpapi")
	assert.Equal(t, span.Meta[RequestID], "FaHnXjKCGjQEJ7A=")
	assert.Equal(t, span.Meta[ResourceNames], "GET ")
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketDefaultEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-default.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	EnrichInferredSpanWithAPIGatewayWebsocketEvent(eventKeys, inferredSpan)

	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1631285061000000000))
	assert.Equal(t, span.Service, "p62c47itsb.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Name, "aws.apigateway.websocket")
	assert.Equal(t, span.Resource, "$default")
	assert.Equal(t, span.Type, "web")
	assert.Equal(t, span.Meta[APIID], "p62c47itsb")
	assert.Equal(t, span.Meta[APIName], "p62c47itsb")
	assert.Equal(t, span.Meta[ConnectionID], "Fc5SzcoYGjQCJlg=")
	assert.Equal(t, span.Meta[Endpoint], "$default")
	assert.Equal(t, span.Meta[HTTPURL], "p62c47itsb.execute-api.sa-east-1.amazonaws.com$default")
	assert.Equal(t, span.Meta[MessageDirection], "IN")
	assert.Equal(t, span.Meta[OperationName], "aws.apigateway.websocket")
	assert.Equal(t, span.Meta[RequestID], "Fc5S3EvdGjQFtsQ=")
	assert.Equal(t, span.Meta[ResourceNames], "$default")
	assert.Equal(t, span.Meta[Stage], "dev")
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketConnectEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-connect.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	EnrichInferredSpanWithAPIGatewayWebsocketEvent(eventKeys, inferredSpan)

	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1631284003000000000))
	assert.Equal(t, span.Service, "p62c47itsb.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Name, "aws.apigateway.websocket")
	assert.Equal(t, span.Resource, "$connect")
	assert.Equal(t, span.Type, "web")
	assert.Equal(t, span.Meta[APIID], "p62c47itsb")
	assert.Equal(t, span.Meta[APIName], "p62c47itsb")
	assert.Equal(t, span.Meta[ConnectionID], "Fc2tgfl3mjQCJfA=")
	assert.Equal(t, span.Meta[Endpoint], "$connect")
	assert.Equal(t, span.Meta[HTTPURL], "p62c47itsb.execute-api.sa-east-1.amazonaws.com$connect")
	assert.Equal(t, span.Meta[MessageDirection], "IN")
	assert.Equal(t, span.Meta[OperationName], "aws.apigateway.websocket")
	assert.Equal(t, span.Meta[RequestID], "Fc2tgH1RmjQFnOg=")
	assert.Equal(t, span.Meta[ResourceNames], "$connect")
	assert.Equal(t, span.Meta[Stage], "dev")
}

func TestEnrichInferredSpanWithAPIGatewayWebsocketDisconnectEvent(t *testing.T) {
	var eventKeys EventKeys
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-disconnect.json"), &eventKeys)
	inferredSpan := mockInferredSpan()
	span := inferredSpan.Span

	EnrichInferredSpanWithAPIGatewayWebsocketEvent(eventKeys, inferredSpan)

	assert.Equal(t, span.TraceID, uint64(7353030974370088224))
	assert.Equal(t, span.SpanID, uint64(8048964810003407541))
	assert.Equal(t, span.Start, int64(1631284034000000000))
	assert.Equal(t, span.Service, "p62c47itsb.execute-api.sa-east-1.amazonaws.com")
	assert.Equal(t, span.Name, "aws.apigateway.websocket")
	assert.Equal(t, span.Resource, "$disconnect")
	assert.Equal(t, span.Type, "web")
	assert.Equal(t, span.Meta[APIID], "p62c47itsb")
	assert.Equal(t, span.Meta[APIName], "p62c47itsb")
	assert.Equal(t, span.Meta[ConnectionID], "Fc2tgfl3mjQCJfA=")
	assert.Equal(t, span.Meta[Endpoint], "$disconnect")
	assert.Equal(t, span.Meta[HTTPURL], "p62c47itsb.execute-api.sa-east-1.amazonaws.com$disconnect")
	assert.Equal(t, span.Meta[MessageDirection], "IN")
	assert.Equal(t, span.Meta[OperationName], "aws.apigateway.websocket")
	assert.Equal(t, span.Meta[RequestID], "Fc2ydE4LmjQFhdg=")
	assert.Equal(t, span.Meta[ResourceNames], "$disconnect")
	assert.Equal(t, span.Meta[Stage], "dev")
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
