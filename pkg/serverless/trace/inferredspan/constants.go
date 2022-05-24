// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"github.com/aws/aws-lambda-go/events"
)

const (
	// APIID and below are used for inferred span
	// tagging and enrichment
	APIID            = "apiid"
	APIName          = "apiname"
	ConnectionID     = "connection_id"
	Endpoint         = "endpoint"
	EventType        = "event_type"
	HTTP             = "http"
	HTTPURL          = "http.url"
	HTTPMethod       = "http.method"
	HTTPProtocol     = "http.protocol"
	HTTPSourceIP     = "http.source_ip"
	HTTPUserAgent    = "http.user_agent"
	MessageDirection = "message_direction"
	MessageID        = "message_id"
	OperationName    = "operation_name"
	RequestID        = "request_id"
	ResourceNames    = "resource_names"
	Stage            = "stage"
	Subject          = "subject"
	TopicName        = "topicname"
	TopicARN         = "topic_arn"
	Type             = "type"
	// APIGATEWAY and below are used for parsing
	// and setting the event sources
	APIGATEWAY = "apigateway"
	HTTPAPI    = "http-api"
	SNS        = "sns"
	SNSType    = "aws:sns"
	WEBSOCKET  = "websocket"
	UNKNOWN    = "unknown"
)

type APIGatewayRestEvent struct {
	events.APIGatewayProxyRequest
}

// type APIGatewayHTTPEvent struct {
// 	events.APIGatewayV2HTTPRequest
// }

// type APIGatewayWebsocketEvent struct {
// 	events.APIGatewayWebsocketProxyRequest
// }

type AlbEvent struct {
	events.ALBTargetGroupRequest
}

type CloudWatchLogsEvent struct {
	events.CloudwatchLogsEvent
}
type CloudWatchEvent struct {
	events.CloudWatchEvent
}

type DynamoDBEvent struct {
	events.DynamoDBEvent
}

//Not in library
type EventBridgeEvent struct {
}

type KinesisEvent struct {
	events.KinesisEvent
}

//Not in Library
type LambdaFunctionURL struct {
	//not sure about this one??
	//    if request_context and request_context.get("stage"):
	//     if "domainName" in request_context and detect_lambda_function_url_domain(
	//         request_context.get("domainName")
	//     ):
	//         return _EventSource(EventTypes.LAMBDA_FUNCTION_URL)
}

type S3Event struct {
	events.S3Event
}
type SNSEvent struct {
	events.SNSEvent
}

type SQSEvent struct {
	events.SQSEvent
}

////////////////////////////////////////////
///////////////// OLD CODE /////////////////
////////////////////////////////////////////
type APIGatewayBaseEvent struct {
	RequestContext RequestContextKeys `mapstructure:"requestContext" json:"requestContext"`
	Headers        HeaderKeys         `mapstructure:"headers" json:"headers"`
}

// APIGatewayRESTEvent is the API gateway request event
type APIGatewayRESTEvent struct {
	APIGatewayBaseEvent
	Path       string `mapstructure:"path" json:"path"`
	HTTPMethod string `mapstructure:"httpMethod" json:"httpMethod"`
}

type APIGatewayHTTPEvent struct {
	APIGatewayBaseEvent
}

type APIGatewayWebsocketEvent struct {
	APIGatewayBaseEvent
}

// SNSRequest is the SNS event
type SNSRequest struct {
	Records []*RecordKeys `mapStructure:"Records" json:"Records"`
}

// EventKeys are used to tell us what event type we received
type EventKeys struct {
	RequestContext RequestContextKeys `json:"requestContext"`
	Headers        HeaderKeys         `json:"headers"`
	Records        []*RecordKeys      `json:"Records"`
	HTTPMethod     string             `json:"httpMethod"`
	Path           string             `json:"path"`
}

// RequestContextKeys holds the nested requestContext from the payload.
type RequestContextKeys struct {
	Stage            string   `json:"stage"`
	RouteKey         string   `json:"routeKey"`
	MessageDirection string   `json:"messageDirection"`
	Domain           string   `json:"domainName"`
	APIID            string   `json:"apiId"`
	RawPath          string   `json:"rawPath"`
	RequestID        string   `json:"requestID"`
	RequestTimeEpoch int64    `json:"requestTimeEpoch"`
	HTTP             HTTPKeys `json:"http"`
	ConnectionID     string   `json:"connectionId"`
	EventType        string   `json:"eventType"`
	TimeEpoch        int64    `json:"timeEpoch"`
}

// HeaderKeys holds the extracted headers from the trace context
type HeaderKeys struct {
	InvocationType string `json:"X-Amz-Invocation-Type",mapstructure:"X-Amz-Invocation-Type"`
}

// HTTPKeys holds the nested HTTP data from the event payload
type HTTPKeys struct {
	Method    string `json:"method"`
	Protocol  string `json:"protocol"`
	SourceIP  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}

// RecordKeys holds the data for Records
type RecordKeys struct {
	EventSource string  `json:"EventSource"`
	SNS         SNSKeys `json:"Sns"`
}

// SNSKeys holds the SNS data
type SNSKeys struct {
	MessageID string  `json:"MessageID"`
	TopicArn  string  `json:"TopicArn"`
	Type      string  `json:"Type"`
	TimeStamp string  `json:"Timestamp"`
	Subject   *string `json:"Subject"`
}
