// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const registerExtensionTimeout = 1 * time.Second

func TestCreateRegistrationPayload(t *testing.T) {
	payload := createRegistrationPayload()
	assert.Equal(t, "{\"events\":[\"INVOKE\", \"SHUTDOWN\"]}", payload.String())
}

func TestExtractID(t *testing.T) {
	expectedID := "blablabla"
	response := &http.Response{
		Header: map[string][]string{
			HeaderExtID: {expectedID},
		},
	}
	assert.Equal(t, expectedID, extractID(response))
}

func TestIsValidResponseTrue(t *testing.T) {
	response := &http.Response{
		StatusCode: 200,
	}
	assert.True(t, isAValidResponse(response))
}

func TestIsValidResponseFalse(t *testing.T) {
	response := &http.Response{
		StatusCode: 404,
	}
	assert.False(t, isAValidResponse(response))
}

func TestBuildRegisterRequestSuccess(t *testing.T) {
	request, err := buildRegisterRequest("X-Header", "extensionName", "myUrl", bytes.NewBuffer([]byte("blablabla")))
	assert.Nil(t, err)
	assert.Equal(t, http.MethodPost, request.Method)
	assert.Equal(t, "myUrl", request.URL.Path)
	assert.NotNil(t, request.Body)
	assert.Equal(t, "extensionName", request.Header["X-Header"][0])
}

func TestBuildRegisterRequestFailure(t *testing.T) {
	request, err := buildRegisterRequest("X-Header", "extensionName", ":invalid:", bytes.NewBuffer([]byte("blablabla")))
	assert.Nil(t, request)
	assert.NotNil(t, err)
}

func TestSendRequestFailure(t *testing.T) {
	response, err := sendRequest(&http.Client{}, &http.Request{})
	if err == nil {
		response.Body.Close()
	}
	assert.Nil(t, response)
	assert.NotNil(t, err)
}

func TestSendRequestSuccess(t *testing.T) {
	response, err := sendRequest(&ClientMock{}, &http.Request{})
	assert.Nil(t, err)
	assert.NotNil(t, response)
	if response.Body != nil {
		response.Body.Close()
	}
}

func TestRegisterSuccess(t *testing.T) {
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//add the extension id
		w.Header().Add(HeaderExtID, "myGeneratedId")
		w.WriteHeader(200)
	}))
	defer ts.Close()

	baseRuntime := strings.Replace(ts.URL, "http://", "", 1)
	t.Setenv("AWS_LAMBDA_RUNTIME_API", baseRuntime)
	id, err := RegisterExtension(baseRuntime, "/myRoute", registerExtensionTimeout)

	assert.Equal(t, "myGeneratedId", id.String())
	assert.Nil(t, err)
}

func TestRegisterErrorNoExtensionId(t *testing.T) {
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//no the extension id
		w.WriteHeader(200)
	}))
	defer ts.Close()

	t.Setenv("AWS_LAMBDA_RUNTIME_API", ts.URL)
	id, err := RegisterExtension(strings.Replace(ts.URL, "http://", "", 1), "", registerExtensionTimeout)

	assert.Empty(t, id.String())
	assert.NotNil(t, err)
}

func TestRegisterErrorHttp(t *testing.T) {
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// non 200 http code
		w.WriteHeader(500)
	}))
	defer ts.Close()

	id, err := RegisterExtension(strings.Replace(ts.URL, "http://", "", 1), "", registerExtensionTimeout)

	assert.Empty(t, id.String())
	assert.NotNil(t, err)
}

func TestRegisterErrorTimeout(t *testing.T) {
	id, err := RegisterExtension(":invalidURL:", "", registerExtensionTimeout)
	assert.Empty(t, id.String())
	assert.NotNil(t, err)
}

func TestRegisterErrorBuildRequest(t *testing.T) {
	//fake the register route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(registerExtensionTimeout + 10*time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	id, err := RegisterExtension(ts.URL, "", registerExtensionTimeout)

	assert.Empty(t, id.String())
	assert.NotNil(t, err)
}

func TestRegisterInvalidUrl(t *testing.T) {
	id, err := RegisterExtension(":inv al id:", "", registerExtensionTimeout)
	assert.Empty(t, id.String())
	assert.NotNil(t, err)
}
