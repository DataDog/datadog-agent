// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractEventSourceUnknown(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET"}}`
	attributes := parseEvent(testString)
	str := attributes.extractEventSource()
	assert.Equal(t, str, UNKNOWN)
}
func TestExtractEventSourceREST(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET", "stage":"dev"}}`
	attributes := parseEvent(testString)
	str := attributes.extractEventSource()
	assert.Equal(t, str, APIGATEWAY)
}
func TestExtractEventSourceHTTP(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"routeKey":"GET /httpapi/get", "stage":"dev"}}`
	attributes := parseEvent(testString)
	str := attributes.extractEventSource()
	assert.Equal(t, str, HTTPAPI)
}
func TestExtractEventSourceWebsocket(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"messageDirection":"IN", "stage":"dev"}}`
	attributes := parseEvent(testString)
	str := attributes.extractEventSource()
	assert.Equal(t, str, WEBSOCKET)
}

func TestExtractEventSourceNoRequeestContext(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET"}`
	attributes := parseEvent(testString)
	str := attributes.extractEventSource()
	assert.Equal(t, str, UNKNOWN)
}
