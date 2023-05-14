// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const registerLogsTimeout = 100 * time.Millisecond

func TestBuildLogRegistrationPayload(t *testing.T) {
	payload := buildLogRegistrationPayload("myUri", "platform function", 10, 100, 1000)
	assert.Equal(t, "HTTP", payload.Destination.Protocol)
	assert.Equal(t, "myUri", payload.Destination.URI)
	assert.Equal(t, 10, payload.Buffering.TimeoutMs)
	assert.Equal(t, 100, payload.Buffering.MaxBytes)
	assert.Equal(t, 1000, payload.Buffering.MaxItems)
	assert.Equal(t, []string{"platform", "function"}, payload.Types)
}

func TestBuildLogRegistrationRequestSuccess(t *testing.T) {
	request, err := buildLogRegistrationRequest("myUrl", "X-Extension", "Content-Type", "myId", []byte("test"))
	assert.Nil(t, err)
	assert.Equal(t, http.MethodPut, request.Method)
	assert.Equal(t, "myUrl", request.URL.Path)
	assert.NotNil(t, request.Body)
	assert.Equal(t, "myId", request.Header["X-Extension"][0])
	assert.Equal(t, "application/json", request.Header["Content-Type"][0])
}

func TestBuildLogRegistrationRequestError(t *testing.T) {
	request, err := buildLogRegistrationRequest(":invalid:", "X-Extension", "Content-Type", "myId", []byte("test"))
	assert.NotNil(t, err)
	assert.Nil(t, request)
}

func TestIsValidHTTPCodeSuccess(t *testing.T) {
	assert.True(t, isValidHTTPCode(200))
	assert.True(t, isValidHTTPCode(202))
	assert.True(t, isValidHTTPCode(204))
}

func TestIsValidHTTPCodeError(t *testing.T) {
	assert.False(t, isValidHTTPCode(300))
	assert.False(t, isValidHTTPCode(404))
	assert.False(t, isValidHTTPCode(400))
}

func TestSendLogRegistrationRequestFailure(t *testing.T) {
	response, err := sendLogRegistrationRequest(&http.Client{}, &http.Request{})
	if err == nil {
		response.Body.Close()
	}
	assert.Nil(t, response)
	assert.NotNil(t, err)
}

func TestSendLogRegistrationRequestSuccess(t *testing.T) {
	response, err := sendLogRegistrationRequest(&ClientMock{}, &http.Request{})
	assert.Nil(t, err)
	assert.NotNil(t, response)
	if response.Body != nil {
		response.Body.Close()
	}
}

func TestSubscribeLogsSuccess(t *testing.T) {
	payload := buildLogRegistrationPayload("myUri", "platform function", 10, 100, 1000)
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()
	err := subscribeTelemetry("myId", ts.URL, registerLogsTimeout, payload)
	assert.Nil(t, err)
}

func TestSubscribeLogsTimeout(t *testing.T) {
	payload := buildLogRegistrationPayload("myUri", "platform function", 10, 100, 1000)
	done := make(chan struct{})
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// timeout
		select {
		case <-time.After(registerLogsTimeout + 5*time.Second):
		case <-done:
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()
	defer close(done)

	err := subscribeTelemetry("myId", ts.URL, registerLogsTimeout, payload)
	assert.NotNil(t, err)
}

func TestSubscribeLogsInvalidHttpCode(t *testing.T) {
	payload := buildLogRegistrationPayload("myUri", "platform function", 10, 100, 1000)
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// invalid code
		w.WriteHeader(500)
	}))
	defer ts.Close()

	err := subscribeTelemetry("myId", ts.URL, registerLogsTimeout, payload)
	assert.NotNil(t, err)
}

func TestSubscribeLogsInvalidUrl(t *testing.T) {
	payload := buildLogRegistrationPayload("myUri", "platform function", 10, 100, 1000)
	err := subscribeTelemetry("myId", ":invalid:", registerLogsTimeout, payload)
	assert.NotNil(t, err)
}

type ImpossibleToMarshall struct{}

func (p *ImpossibleToMarshall) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("should fail")
}

func TestSubscribeLogsInvalidPayloadObject(t *testing.T) {
	payload := &ImpossibleToMarshall{}
	err := subscribeTelemetry("myId", ":invalid:", registerLogsTimeout, payload)
	assert.NotNil(t, err)
}

func TestGetLogTypesToSubscribeEmpty(t *testing.T) {
	result := getLogTypesToSubscribe("")
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "platform", result[0])
	assert.Equal(t, "function", result[1])
	assert.Equal(t, "extension", result[2])
}

func TestGetLogTypesToSubscribeInvalid(t *testing.T) {
	result := getLogTypesToSubscribe("invalid log type")
	assert.Empty(t, result)
}

func TestGetLogTypesToSubscribeValid(t *testing.T) {
	result := getLogTypesToSubscribe("function blabla extension")
	assert.Equal(t, 2, len(result))
	assert.Equal(t, "function", result[0])
}

func TestBuildCallbackURI(t *testing.T) {
	assert.Equal(t, "http://sandbox:1234/myPath", buildCallbackURI(1234, "/myPath"))
}

func TestEnableTelemetryCollection(t *testing.T) {
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	err := EnableTelemetryCollection(
		EnableTelemetryCollectionArgs{
			ID:                  "myId",
			RegistrationURL:     ts.URL,
			RegistrationTimeout: registerLogsTimeout,
			LogsType:            "platform function",
			Port:                1234,
			CollectionRoute:     "/route",
			Timeout:             10,
			MaxBytes:            100,
			MaxItems:            1000,
		})
	assert.Nil(t, err)
}
