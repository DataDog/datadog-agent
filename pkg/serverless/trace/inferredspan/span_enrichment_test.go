// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
)

const (
	dataFile = "../testdata/event_samples/"
)

// TestGetServiceMapping checks if the function correctly parses the input string into a map.
func TestCreateServiceMapping(t *testing.T) {
	// Case 1: Normal case
	testString := "test1:val1,test2:val2"
	expectedOutput := map[string]string{
		"test1": "val1",
		"test2": "val2",
	}

	result := CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "NewServiceMapping failed, expected %v, got %v", expectedOutput, result)

	// Case 2: Test incorrect format.
	testString = "test1-val1,test2=val2"
	expectedOutput = map[string]string{}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with incorrect format, expected %v, got %v", expectedOutput, result)

	// Case 3: Test same key-value pairs.
	testString = "api1:api1,api2:api2"
	expectedOutput = map[string]string{}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with same key-value pairs, expected %v, got %v", expectedOutput, result)

	// Case 4: Test empty keys.
	testString = ":api1,api2:service2"
	expectedOutput = map[string]string{
		"api2": "service2",
	}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with empty keys, expected %v, got %v", expectedOutput, result)

	// Case 5: Test empty values.
	testString = "api1:,api2:service2"
	expectedOutput = map[string]string{
		"api2": "service2",
	}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with empty values, expected %v, got %v", expectedOutput, result)

	// Case 6: Test more than one colon in the entry.
	testString = "api1:val1:val2,api2:service2"
	expectedOutput = map[string]string{
		"api2": "service2",
	}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with more than one colon, expected %v, got %v", expectedOutput, result)

	// Case 7: Test an empty string.
	testString = ""
	expectedOutput = map[string]string{}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with an empty string, expected %v, got %v", expectedOutput, result)

	// Case 8: Test string with leading and trailing commas.
	testString = ",api1:service1,"
	expectedOutput = map[string]string{
		"api1": "service1",
	}
	result = CreateServiceMapping(testString)
	assert.True(t, reflect.DeepEqual(result, expectedOutput), "CreateServiceMapping failed with leading and trailing commas, expected %v, got %v", expectedOutput, result)
}

// TestDetermineServiceName checks if the function correctly selects a service name from the map.
func TestDetermineServiceName(t *testing.T) {
	serviceMapping := map[string]string{
		"specificKey": "specificVal",
		"genericKey":  "genericVal",
	}
	defaultVal := "defaultVal"

	// Test when the specific key exists.
	expectedOutput := "specificVal"
	result := DetermineServiceName(serviceMapping, "specificKey", "nonexistent", defaultVal)
	assert.Equal(t, expectedOutput, result, "DetermineServiceName failed, expected %v, got %v", expectedOutput, result)

	// Test when only the generic key exists.
	expectedOutput = "genericVal"
	result = DetermineServiceName(serviceMapping, "nonexistent", "genericKey", defaultVal)
	assert.Equal(t, expectedOutput, result, "DetermineServiceName failed, expected %v, got %v", expectedOutput, result)

	// Test when neither key exists.
	expectedOutput = defaultVal
	result = DetermineServiceName(serviceMapping, "nonexistent", "nonexistent", defaultVal)
	assert.Equal(t, expectedOutput, result, "DetermineServiceName failed, expected %v, got %v", expectedOutput, result)
}

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

func TestRemapsAllInferredSpanServiceNamesFromAPIGatewayEvent(t *testing.T) {
	// Load the original event
	var apiGatewayRestEvent events.APIGatewayProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway.json"), &apiGatewayRestEvent)

	inferredSpan := mockInferredSpan()
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	newServiceMapping := map[string]string{
		"apiId_from_event":   "ignored-name",
		"lambda_api_gateway": "accepted-name",
	}
	// Set up test case
	SetServiceMapping(newServiceMapping)
	inferredSpan.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.apigateway.rest", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	apiGatewayRestEvent2 := apiGatewayRestEvent
	apiGatewayRestEvent2.RequestContext.DomainName = "different.execute-api.us-east-2.amazonaws.com"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.apigateway.rest", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromAPIGatewayEvent(t *testing.T) {
	// Load the original event
	var apiGatewayRestEvent events.APIGatewayProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway.json"), &apiGatewayRestEvent)
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"1234567890": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.apigateway.rest", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	apiGatewayRestEvent2 := apiGatewayRestEvent
	apiGatewayRestEvent2.RequestContext.APIID = "different"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithAPIGatewayRESTEvent(apiGatewayRestEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.apigateway.rest", span2.Meta[operationName])
	assert.Equal(t, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com", span2.Service)
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

func TestEnrichInferredSpanWithLambdaFunctionURLEventt(t *testing.T) {
	var apiGatewayHTTPEvent events.LambdaFunctionURLRequest
	_ = json.Unmarshal(getEventFromFile("http-api.json"), &apiGatewayHTTPEvent)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithLambdaFunctionURLEvent(apiGatewayHTTPEvent)

	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, int64(1631212283738000000), span.Start)
	assert.Equal(t, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com", span.Service)
	assert.Equal(t, "aws.lambda.url", span.Name)
	assert.Equal(t, "GET /httpapi/get", span.Resource)
	assert.Equal(t, "http", span.Type)
	assert.Equal(t, "GET", span.Meta[httpMethod])
	assert.Equal(t, "HTTP/1.1", span.Meta[httpProtocol])
	assert.Equal(t, "38.122.226.210", span.Meta[httpSourceIP])
	assert.Equal(t, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com/httpapi/get", span.Meta[httpURL])
	assert.Equal(t, "curl/7.64.1", span.Meta[httpUserAgent])
	assert.Equal(t, "aws.lambda.url", span.Meta[operationName])
	assert.Equal(t, "FaHnXjKCGjQEJ7A=", span.Meta[requestID])
	assert.Equal(t, "GET /httpapi/get", span.Meta[resourceNames])
}

func TestRemapsSpecificInferredSpanServiceNamesFromAPIGatewayHTTPAPIEvent(t *testing.T) {
	// Load the original event
	var apiGatewayHTTPAPIEvent events.APIGatewayV2HTTPRequest
	_ = json.Unmarshal(getEventFromFile("http-api.json"), &apiGatewayHTTPAPIEvent)
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"x02yirxc7a": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayHTTPEvent(apiGatewayHTTPAPIEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.httpapi", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	apiGatewayHTTPAPIEvent2 := apiGatewayHTTPAPIEvent
	apiGatewayHTTPAPIEvent2.RequestContext.APIID = "different"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithAPIGatewayHTTPEvent(apiGatewayHTTPAPIEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.httpapi", span2.Meta[operationName])
	assert.Equal(t, "x02yirxc7a.execute-api.sa-east-1.amazonaws.com", span2.Service)
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

func TestRemapsSpecificInferredSpanServiceNamesFromAPIGatewayWebsocketDefaultEvent(t *testing.T) {
	// Load the original event
	var apiGatewayWebsocketEvent events.APIGatewayWebsocketProxyRequest
	_ = json.Unmarshal(getEventFromFile("api-gateway-websocket-default.json"), &apiGatewayWebsocketEvent)
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"p62c47itsb": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithAPIGatewayWebsocketEvent(apiGatewayWebsocketEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.apigateway.websocket", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	apiGatewayWebsocketEvent2 := apiGatewayWebsocketEvent
	apiGatewayWebsocketEvent2.RequestContext.APIID = "different"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithAPIGatewayWebsocketEvent(apiGatewayWebsocketEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.apigateway.websocket", span2.Meta[operationName])
	assert.Equal(t, "p62c47itsb.execute-api.sa-east-1.amazonaws.com", span2.Service)
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

func TestRemapsAllInferredSpanServiceNamesFromSNSEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_sns": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	// Load the original event
	var snsEvent events.SNSEvent
	_ = json.Unmarshal(getEventFromFile("sns.json"), &snsEvent)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSNSEvent(snsEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.sns", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	snsEvent2 := snsEvent
	snsEvent2.Records[0].SNS.TopicArn = "arn:aws:sns:us-east-2:123456789012:different"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithSNSEvent(snsEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.sns", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromSNSEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"serverlessTracingTopicPy": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var snsEvent events.SNSEvent
	_ = json.Unmarshal(getEventFromFile("sns.json"), &snsEvent)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSNSEvent(snsEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.sns", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	snsEvent2 := snsEvent
	snsEvent2.Records[0].SNS.TopicArn = "arn:aws:sns:us-east-2:123456789012:different"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithSNSEvent(snsEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.sns", span2.Meta[operationName])
	assert.Equal(t, "sns", span2.Service)
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

func TestRemapsAllInferredSpanServiceNamesFromS3Event(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_s3": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	// Load the original event
	var s3Event events.S3Event
	_ = json.Unmarshal(getEventFromFile("s3.json"), &s3Event)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithS3Event(s3Event)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.s3", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	s3Event2 := s3Event
	s3Event2.Records[0].S3.Bucket.Arn = "arn:aws:s3:::different-example-bucket"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithS3Event(s3Event2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.s3", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromS3Event(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"example-bucket": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)

	// Load the original event
	var s3Event events.S3Event
	_ = json.Unmarshal(getEventFromFile("s3.json"), &s3Event)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithS3Event(s3Event)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.s3", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	s3Event2 := s3Event
	s3Event2.Records[0].S3.Bucket.Name = "different-example-bucket"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithS3Event(s3Event2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.s3", span2.Meta[operationName])
	assert.Equal(t, "s3", span2.Service)
}

func TestEnrichInferredSpanWithEventBridgeEvent(t *testing.T) {
	var eventBridgeEvent events.EventBridgeEvent
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
	assert.Equal(t, "testdetail", span.Meta[detailType])
	assert.True(t, inferredSpan.IsAsync)
}

func TestRemapsAllInferredSpanServiceNamesFromEventBridgeEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_eventbridge": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var eventBridgeEvent events.EventBridgeEvent
	_ = json.Unmarshal(getEventFromFile("eventbridge-custom.json"), &eventBridgeEvent)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithEventBridgeEvent(eventBridgeEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.eventbridge", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	eventBridgeEvent2 := eventBridgeEvent
	eventBridgeEvent2.Source = "different.event.sender"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithEventBridgeEvent(eventBridgeEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.eventbridge", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromEventBridgeEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"eventbridge.custom.event.sender": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var eventBridgeEvent events.EventBridgeEvent
	_ = json.Unmarshal(getEventFromFile("eventbridge-custom.json"), &eventBridgeEvent)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithEventBridgeEvent(eventBridgeEvent)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.eventbridge", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	eventBridgeEvent2 := eventBridgeEvent
	eventBridgeEvent2.Source = "different.event.sender"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithEventBridgeEvent(eventBridgeEvent2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.eventbridge", span2.Meta[operationName])
	assert.Equal(t, "eventbridge", span2.Service)
}

func TestEnrichInferredSpanWithSQSEvent(t *testing.T) {
	var sqsRequest events.SQSEvent
	_ = json.Unmarshal(getEventFromFile("sqs.json"), &sqsRequest)
	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSQSEvent(sqsRequest)
	span := inferredSpan.Span
	assert.Equal(t, uint64(7353030974370088224), span.TraceID)
	assert.Equal(t, uint64(8048964810003407541), span.SpanID)
	assert.Equal(t, uint64(0), span.ParentID)
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

func TestRemapsAllInferredSpanServiceNamesFromSQSEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_sqs": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var sqsRequest events.SQSEvent
	_ = json.Unmarshal(getEventFromFile("sqs.json"), &sqsRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSQSEvent(sqsRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.sqs", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	sqsRequest2 := sqsRequest
	sqsRequest2.Records[0].EventSourceARN = "arn:aws:sqs:sa-east-1:425362996713:differentQueue"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithSQSEvent(sqsRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.sqs", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromSQSEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"InferredSpansQueueNode": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var sqsRequest events.SQSEvent
	_ = json.Unmarshal(getEventFromFile("sqs.json"), &sqsRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithSQSEvent(sqsRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.sqs", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	sqsRequest2 := sqsRequest
	sqsRequest2.Records[0].EventSourceARN = "arn:aws:sqs:sa-east-1:425362996713:differentQueue"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithSQSEvent(sqsRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.sqs", span2.Meta[operationName])
	assert.Equal(t, "sqs", span2.Service)
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
	assert.Equal(t, "kinesisStream", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, "aws.kinesis", span.Meta[operationName])
	assert.Equal(t, "kinesisStream", span.Meta[resourceNames])
	assert.Equal(t, "kinesisStream", span.Meta[streamName])
	assert.Equal(t, "shardId-000000000002", span.Meta[shardID])
	assert.Equal(t, "arn:aws:kinesis:sa-east-1:425362996713:stream/kinesisStream", span.Meta[eventSourceArn])
	assert.Equal(t, "shardId-000000000002:49624230154685806402418173680709770494154422022871973922", span.Meta[eventID])
	assert.Equal(t, "aws:kinesis:record", span.Meta[eventName])
	assert.Equal(t, "1.0", span.Meta[eventVersion])
	assert.Equal(t, "partitionkey", span.Meta[partitionKey])
	assert.True(t, inferredSpan.IsAsync)
}

func TestRemapsAllInferredSpanServiceNamesFromKinesisEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_kinesis": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var kinesisRequest events.KinesisEvent
	_ = json.Unmarshal(getEventFromFile("kinesis.json"), &kinesisRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithKinesisEvent(kinesisRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.kinesis", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	kinesisRequest2 := kinesisRequest
	kinesisRequest2.Records[0].EventSourceArn = "arn:aws:kinesis:sa-east-1:425362996713:differentStream"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithKinesisEvent(kinesisRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.kinesis", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromKinesisEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"kinesisStream": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var kinesisRequest events.KinesisEvent
	_ = json.Unmarshal(getEventFromFile("kinesis.json"), &kinesisRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithKinesisEvent(kinesisRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.kinesis", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	kinesisRequest2 := kinesisRequest
	kinesisRequest2.Records[0].EventSourceArn = "arn:aws:kinesis:sa-east-1:425362996713:stream/differentKinesisStream"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithKinesisEvent(kinesisRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.kinesis", span2.Meta[operationName])
	assert.Equal(t, "kinesis", span2.Service)
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

func TestRemapsAllInferredSpanServiceNamesFromDynamoDBEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"lambda_dynamodb": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var dynamoRequest events.DynamoDBEvent
	_ = json.Unmarshal(getEventFromFile("dynamodb.json"), &dynamoRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithDynamoDBEvent(dynamoRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.dynamodb", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	dynamoRequest2 := dynamoRequest
	dynamoRequest2.Records[0].EventSourceArn = "arn:aws:dynamodb:us-east-1:123456789012:table/DifferentTable/stream/2015-06-27T00:48:05.899"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithDynamoDBEvent(dynamoRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.dynamodb", span2.Meta[operationName])
	assert.Equal(t, "accepted-name", span2.Service)
}

func TestRemapsSpecificInferredSpanServiceNamesFromDynamoDBEvent(t *testing.T) {
	// Store the original service mapping
	origServiceMapping := GetServiceMapping()

	// Clean up: Reset the service mapping to its original state after this test
	defer func() {
		SetServiceMapping(origServiceMapping)
	}()
	// Set up the service mapping
	newServiceMapping := map[string]string{
		"ExampleTableWithStream": "accepted-name",
	}
	SetServiceMapping(newServiceMapping)
	// Load the original event
	var dynamoRequest events.DynamoDBEvent
	_ = json.Unmarshal(getEventFromFile("dynamodb.json"), &dynamoRequest)

	inferredSpan := mockInferredSpan()
	inferredSpan.EnrichInferredSpanWithDynamoDBEvent(dynamoRequest)

	span1 := inferredSpan.Span
	assert.Equal(t, "aws.dynamodb", span1.Meta[operationName])
	assert.Equal(t, "accepted-name", span1.Service)

	// Create a copy of the original event and modify it
	dynamoRequest2 := dynamoRequest
	dynamoRequest2.Records[0].EventSourceArn = "arn:aws:dynamodb:us-east-1:123456789012:table/DifferentTable/stream/2015-06-27T00:48:05.899"

	inferredSpan2 := mockInferredSpan()
	inferredSpan2.EnrichInferredSpanWithDynamoDBEvent(dynamoRequest2)

	span2 := inferredSpan2.Span
	assert.Equal(t, "aws.dynamodb", span2.Meta[operationName])
	assert.Equal(t, "dynamodb", span2.Service)
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
