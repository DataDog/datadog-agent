// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEventSourceUnknown(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, UNKNOWN)
}
func TestParseEventSourceREST(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, APIGATEWAY)
}
func TestParseEventSourceHTTP(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"routeKey":"GET /httpapi/get", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, HTTPAPI)
}
func TestParseEventSourceWebsocket(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"messageDirection":"IN", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, WEBSOCKET)
}
