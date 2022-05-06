// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

// Used for inferred span tagging and enrichment
const (
	OperationName    = "operation_name"
	HTTP             = "http"
	HTTPURL          = "http.url"
	HTTPMethod       = "http.method"
	HTTPProtocol     = "http.protocol"
	HTTPSourceIP     = "http.source_ip"
	HTTPUserAgent    = "http.user_agent"
	Endpoint         = "endpoint"
	ResourceNames    = "resource_names"
	APIID            = "apiid"
	APIName          = "apiname"
	Stage            = "stage"
	RequestID        = "request_id"
	ConnectionID     = "connection_id"
	EventType        = "event_type"
	MessageDirection = "message_direction"
	APIGATEWAY       = "apigateway"
	HTTPAPI          = "http-api"
	WEBSOCKET        = "websocket"
	UNKNOWN          = "unknown"
)

// EventKeys are used to tell us what event type we received
type EventKeys struct {
	RequestContext RequestContextKeys `json:"requestContext"`
	Headers        HeaderKeys         `json:"headers"`
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
	InvocationType string `json:"X-Amz-Invocation-Type"`
}

// HTTPKeys holds the nested HTTP data from the event payload
type HTTPKeys struct {
	Method    string `json:"method"`
	Protocol  string `json:"protocol"`
	SourceIP  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}
