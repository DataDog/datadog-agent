// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package trigger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventPayloadParsing(t *testing.T) {
	testDir := "./testData"
	testCases := map[string]eventParseFunc{
		"api-gateway-authorizer-request.json": isAPIGatewayLambdaAuthorizerRequestParametersEvent,
		"api-gateway-authorizer-token.json":   isAPIGatewayLambdaAuthorizerTokenEvent,
		"api-gateway-v1.json":                 isAPIGatewayEvent,
		"api-gateway-v2.json":                 isAPIGatewayV2Event,
		"api-gateway-kong.json":               isKongAPIGatewayEvent,
		"application-load-balancer.json":      isALBEvent,
		"cloudwatch-events.json":              isCloudwatchEvent,
		"cloudwatch-logs.json":                isCloudwatchLogsEvent,
		"cloudfront.json":                     isCloudFrontRequestEvent,
		"dynamodb.json":                       isDynamoDBStreamEvent,
		"eventbridge-custom.json":             isEventBridgeEvent,
		"kinesis.json":                        isKinesisStreamEvent,
		"s3.json":                             isS3Event,
		"sns.json":                            isSNSEvent,
		"sqs.json":                            isSQSEvent,
		"lambdaurl.json":                      isLambdaFunctionURLEvent,
	}
	for testFile, testFunc := range testCases {
		file, err := os.Open(fmt.Sprintf("%v/%v", testDir, testFile))
		assert.NoError(t, err)

		jsonData, err := io.ReadAll(file)
		assert.NoError(t, err)

		event, err := Unmarshal(bytes.ToLower(jsonData))
		assert.NoError(t, err)

		funcName := runtime.FuncForPC(reflect.ValueOf(testFunc).Pointer()).Name()
		assert.True(t, testFunc(event), fmt.Sprintf("Test: %v, %v", testFile, funcName))
	}
}

func TestEventPayloadParsingWrong(t *testing.T) {
	testDir := "./testData"
	testCases := map[string]eventParseFunc{
		"api-gateway-authorizer-request.json": isAPIGatewayLambdaAuthorizerRequestParametersEvent,
		"api-gateway-authorizer-token.json":   isAPIGatewayLambdaAuthorizerTokenEvent,
		"api-gateway-v1.json":                 isAPIGatewayEvent,
		"api-gateway-v2.json":                 isAPIGatewayV2Event,
		"application-load-balancer.json":      isALBEvent,
		"cloudwatch-events.json":              isCloudwatchEvent,
		"cloudwatch-logs.json":                isCloudwatchLogsEvent,
		"cloudfront.json":                     isCloudFrontRequestEvent,
		"dynamodb.json":                       isDynamoDBStreamEvent,
		"eventbridge-custom.json":             isEventBridgeEvent,
		"kinesis.json":                        isKinesisStreamEvent,
		"s3.json":                             isS3Event,
		"sns.json":                            isSNSEvent,
		"sqs.json":                            isSQSEvent,
		"lambdaurl.json":                      isLambdaFunctionURLEvent,
	}
	for correctTestFile, testFunc := range testCases {
		wrongTestFiles, err := os.ReadDir(testDir)
		assert.NoError(t, err)

		for _, wrongTestFile := range wrongTestFiles {
			if correctTestFile == wrongTestFile.Name() {
				// skip testing the correct case
				continue
			}
			file, err := os.Open(fmt.Sprintf("%v/%v", testDir, wrongTestFile.Name()))
			assert.NoError(t, err)

			jsonData, err := io.ReadAll(file)
			assert.NoError(t, err)

			event, err := Unmarshal(bytes.ToLower(jsonData))
			assert.NoError(t, err)

			funcName := runtime.FuncForPC(reflect.ValueOf(testFunc).Pointer()).Name()
			assert.False(t, testFunc(event), fmt.Sprintf("Test: %v, %v", wrongTestFile, funcName))
		}
	}
}

func TestGetEventType(t *testing.T) {
	testDir := "./testData"
	testCases := map[string]AWSEventType{
		"api-gateway-authorizer-request.json": APIGatewayLambdaAuthorizerRequestParametersEvent,
		"api-gateway-authorizer-token.json":   APIGatewayLambdaAuthorizerTokenEvent,
		"api-gateway-v1.json":                 APIGatewayEvent,
		"api-gateway-v2.json":                 APIGatewayV2Event,
		"api-gateway-kong.json":               APIGatewayEvent,
		"application-load-balancer.json":      ALBEvent,
		"cloudwatch-events.json":              CloudWatchEvent,
		"cloudwatch-logs.json":                CloudWatchLogsEvent,
		"cloudfront.json":                     CloudFrontRequestEvent,
		"dynamodb.json":                       DynamoDBStreamEvent,
		"eventbridge-custom.json":             EventBridgeEvent,
		"kinesis.json":                        KinesisStreamEvent,
		"s3.json":                             S3Event,
		"sns.json":                            SNSEvent,
		"sqs.json":                            SQSEvent,
		"lambdaurl.json":                      LambdaFunctionURLEvent,
	}

	for testFile, expectedEventType := range testCases {
		t.Run(testFile, func(t *testing.T) {
			file, err := os.Open(fmt.Sprintf("%v/%v", testDir, testFile))
			assert.NoError(t, err)

			jsonData, err := io.ReadAll(file)
			assert.NoError(t, err)

			jsonPayload, err := Unmarshal(bytes.ToLower(jsonData))
			assert.NoError(t, err)

			parsedEventType := GetEventType(jsonPayload)

			assert.Equal(t, expectedEventType, parsedEventType)

			lastEventChecker = unknownChecker // reset event check cache
		})
	}
}

func TestUnknownChecker(t *testing.T) {
	assert.Nil(t, unknownChecker.check)
	assert.Equal(t, Unknown, unknownChecker.typ)
}

func TestGetEventTypeCaching(t *testing.T) {
	payloadFiles, err := os.ReadDir("../trace/testdata/event_samples")
	if err != nil {
		t.Fatal(err)
	}
	type testcase struct {
		name    string
		payload map[string]any
		result  AWSEventType
	}
	testcases := make([]testcase, 0, len(payloadFiles))
	for _, file := range payloadFiles {
		raw, err := os.ReadFile("../trace/testdata/event_samples/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		err = json.Unmarshal(bytes.ToLower(raw), &payload)
		if err != nil {
			t.Fatal(err)
		}
		result := GetEventType(payload)
		testcases = append(testcases, testcase{file.Name(), payload, result})
	}

	for _, test1 := range testcases {
		for _, test2 := range testcases {
			t.Run(test1.name+"/"+test2.name, func(t *testing.T) {
				lastEventChecker = unknownChecker
				assert.Equal(t, test1.result, GetEventType(test1.payload))
				assert.Equal(t, test2.result, GetEventType(test2.payload))
			})
		}
	}
}
