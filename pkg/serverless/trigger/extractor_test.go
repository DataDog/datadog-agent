// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package trigger

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
)

func TestGetAWSPartitionByRegion(t *testing.T) {
	assert.Equal(t, "aws", GetAWSPartitionByRegion("us-east-1"))
	assert.Equal(t, "aws-cn", GetAWSPartitionByRegion("cn-east-1"))
	assert.Equal(t, "aws-us-gov", GetAWSPartitionByRegion("us-gov-west-1"))
}

func TestExtractAPIGatewayEventARN(t *testing.T) {
	region := "us-east-1"
	event := events.APIGatewayProxyRequest{
		RequestContext: events.APIGatewayProxyRequestContext{
			APIID: "test-id",
			Stage: "test-stage",
		},
	}

	arn := ExtractAPIGatewayEventARN(event, region)
	assert.Equal(t, "arn:aws:apigateway:us-east-1::/restapis/test-id/stages/test-stage", arn)
}

func TestExtractAPIGatewayV2EventARN(t *testing.T) {
	region := "us-east-1"
	event := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			APIID: "test-id",
			Stage: "test-stage",
		},
	}

	arn := ExtractAPIGatewayV2EventARN(event, region)
	assert.Equal(t, "arn:aws:apigateway:us-east-1::/restapis/test-id/stages/test-stage", arn)
}

func TestExtractAPIGatewayWebSocketEventARN(t *testing.T) {
	region := "us-east-1"
	event := events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			APIID: "test-id",
			Stage: "test-stage",
		},
	}

	arn := ExtractAPIGatewayWebSocketEventARN(event, region)
	assert.Equal(t, "arn:aws:apigateway:us-east-1::/restapis/test-id/stages/test-stage", arn)
}

func TestExtractAlbEventARN(t *testing.T) {
	event := events.ALBTargetGroupRequest{
		RequestContext: events.ALBTargetGroupRequestContext{
			ELB: events.ELBContext{
				TargetGroupArn: "test-arn",
			},
		},
	}

	arn := ExtractAlbEventARN(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractCloudwatchEventARN(t *testing.T) {
	event := events.CloudWatchEvent{
		Resources: []string{
			"test-arn",
			"test-arn-2",
		},
	}

	arn := ExtractCloudwatchEventARN(event)
	assert.Equal(t, "test-arn", arn)

	eventEmptyResources := events.CloudWatchEvent{
		Resources: []string{},
	}

	arnEmpty := ExtractCloudwatchEventARN(eventEmptyResources)
	assert.Equal(t, "", arnEmpty)
}

func TestExtractCloudwatchLogsEventARN(t *testing.T) {
	region := "us-east-1"
	accountID := "account-id"
	event := events.CloudwatchLogsEvent{
		AWSLogs: events.CloudwatchLogsRawData{
			Data: "invalid",
		},
	}

	arn, err := ExtractCloudwatchLogsEventARN(event, region, accountID)
	assert.Error(t, err)
	assert.Equal(t, "", arn)

	decodedStr := []byte(`{"logGroup": "testLogGroup"}`)

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, err = gz.Write(decodedStr)

	assert.NoError(t, err)
	assert.NoError(t, gz.Close())

	event = events.CloudwatchLogsEvent{
		AWSLogs: events.CloudwatchLogsRawData{
			Data: base64.StdEncoding.EncodeToString(b.Bytes()),
		},
	}

	arn, err = ExtractCloudwatchLogsEventARN(event, region, accountID)
	assert.Nil(t, err)
	assert.Equal(t, "arn:aws:logs:us-east-1:account-id:log-group:testLogGroup", arn)
}

func TestExtractDynamoDBStreamEventARN(t *testing.T) {
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventSourceArn: "test-arn",
			},
			{
				EventSourceArn: "test-arn2",
			},
		},
	}

	arn := ExtractDynamoDBStreamEventARN(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractKinesisStreamEventARN(t *testing.T) {
	event := events.KinesisEvent{
		Records: []events.KinesisEventRecord{
			{
				EventSourceArn: "test-arn",
			},
			{
				EventSourceArn: "test-arn2",
			},
		},
	}

	arn := ExtractKinesisStreamEventARN(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractS3EventArn(t *testing.T) {
	event := events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventSource: "test-arn",
			},
			{
				EventSource: "test-arn2",
			},
		},
	}

	arn := ExtractS3EventArn(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractSNSEventArn(t *testing.T) {
	event := events.SNSEvent{
		Records: []events.SNSEventRecord{
			{
				SNS: events.SNSEntity{
					TopicArn: "test-arn",
				},
			},
			{
				SNS: events.SNSEntity{
					TopicArn: "test-arn2",
				},
			},
		},
	}

	arn := ExtractSNSEventArn(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractSQSEventARN(t *testing.T) {
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{
				EventSourceARN: "test-arn",
			},
			{
				EventSourceARN: "test-arn2",
			},
		},
	}

	arn := ExtractSQSEventARN(event)
	assert.Equal(t, "test-arn", arn)
}

func TestExtractFunctionURLEventARN(t *testing.T) {
	event := events.APIGatewayProxyRequest{
		Headers: map[string]string{
			"key":     "val",
			"Referer": "referer",
		},
		RequestContext: events.APIGatewayProxyRequestContext{
			DomainName: "domain-name",
			Path:       "path",
			HTTPMethod: "http-method",
		},
	}

	httpTags := GetTagsFromAPIGatewayEvent(event)

	assert.Equal(t, map[string]string{
		"http.url":              "domain-name",
		"http.url_details.path": "path",
		"http.method":           "http-method",
		"http.referer":          "referer",
	}, httpTags)
}

func TestGetTagsFromAPIGatewayEvent(t *testing.T) {
	event := events.APIGatewayProxyRequest{
		Headers: map[string]string{
			"key":     "val",
			"Referer": "referer",
		},
		RequestContext: events.APIGatewayProxyRequestContext{
			DomainName: "domain-name",
			Path:       "path",
			HTTPMethod: "http-method",
		},
	}

	httpTags := GetTagsFromAPIGatewayEvent(event)

	assert.Equal(t, map[string]string{
		"http.url":              "domain-name",
		"http.url_details.path": "path",
		"http.method":           "http-method",
		"http.referer":          "referer",
	}, httpTags)
}

func TestGetTagsFromAPIGatewayV2HTTPRequestNoReferer(t *testing.T) {
	event := events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{
			"key": "val",
		},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			DomainName: "domain-name",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Path:   "path",
				Method: "http-method",
			},
		},
	}

	httpTags := GetTagsFromAPIGatewayV2HTTPRequest(event)

	assert.Equal(t, map[string]string{
		"http.url":              "domain-name",
		"http.url_details.path": "path",
		"http.method":           "http-method",
	}, httpTags)
}

func TestGetTagsFromALBTargetGroupRequest(t *testing.T) {
	event := events.ALBTargetGroupRequest{
		Headers: map[string]string{
			"key":     "val",
			"Referer": "referer",
		},
		Path:       "path",
		HTTPMethod: "http-method",
	}

	httpTags := GetTagsFromALBTargetGroupRequest(event)

	assert.Equal(t, map[string]string{
		"http.url_details.path": "path",
		"http.method":           "http-method",
		"http.referer":          "referer",
	}, httpTags)
}

func TestGetTagsFromFunctionURLRequest(t *testing.T) {
	event := events.LambdaFunctionURLRequest{
		Headers: map[string]string{
			"key":     "val",
			"Referer": "referer",
		},
		RequestContext: events.LambdaFunctionURLRequestContext{
			DomainName: "test-domain",
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Path:   "asd",
				Method: "GET",
			},
		},
	}

	httpTags := GetTagsFromLambdaFunctionURLRequest(event)

	assert.Equal(t, map[string]string{
		"http.url_details.path": "asd",
		"http.method":           "GET",
		"http.referer":          "referer",
		"http.url":              "test-domain",
	}, httpTags)
}

func TestExtractStatusCodeFromHTTPResponse(t *testing.T) {
	noStatusCodePayload := []byte(`{}`)

	statusCode, _ := GetStatusCodeFromHTTPResponse(noStatusCodePayload)
	assert.Equal(t, "", statusCode)

	malformedPayload := []byte(`a`)

	statusCode, _ = GetStatusCodeFromHTTPResponse(malformedPayload)
	assert.Equal(t, "", statusCode)

	statusCodePayload := []byte(`{"statusCode":200}`)

	statusCode, err := GetStatusCodeFromHTTPResponse(statusCodePayload)
	assert.NoError(t, err)
	assert.Equal(t, "200", statusCode)

	statusCodePayloadStr := []byte(`{"statusCode":"200"}`)

	statusCode, _ = GetStatusCodeFromHTTPResponse(statusCodePayloadStr)
	assert.Equal(t, "200", statusCode)
}
