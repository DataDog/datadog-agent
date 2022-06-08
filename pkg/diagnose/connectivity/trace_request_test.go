// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/stretchr/testify/assert"
)

var (
	apiKey                    = "api_key1"
	endpointInfoWithAPIKey    = endpointInfo{Endpoint: endpoints.V1ValidateEndpoint, APIKeyInQueryString: true}
	endpointInfoWithoutAPIKey = endpointInfo{Endpoint: endpoints.V1SeriesEndpoint, APIKeyInQueryString: false}
)

func TestCreateEndpointUrl(t *testing.T) {

	urlWithAPIKey := createEndpointURL("https://domain", endpointInfoWithAPIKey, apiKey)
	urlWithoutAPIKey := createEndpointURL("https://domain2", endpointInfoWithoutAPIKey, apiKey)

	assert.Equal(t, urlWithAPIKey, "https://domain/api/v1/validate?api_key=api_key1")
	assert.Equal(t, urlWithoutAPIKey, "https://domain2/api/v1/series")
}

func TestSendHTTPRequestToEndpoint(t *testing.T) {

	// Create a fake server that send a 200 Response if there is 'api_key1' in the query string
	// or a 400 response otherwise.
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("api_key") == "api_key1" {
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Bad Request"))
		}
	}))
	defer ts1.Close()

	client := forwarder.NewHTTPClient()

	// With the API Key, it should be a 200
	statusCodeWithKey, responseBodyWithKey, errWithKey := sendHTTPRequestToEndpoint(context.Background(), client, ts1.URL, endpointInfoWithAPIKey, apiKey)
	assert.Nil(t, errWithKey)
	assert.Equal(t, statusCodeWithKey, 200)
	assert.Equal(t, string(responseBodyWithKey), "OK")

	// Without the API Key, it should be a 400
	statusCode, responseBody, err := sendHTTPRequestToEndpoint(context.Background(), client, ts1.URL, endpointInfoWithoutAPIKey, apiKey)
	assert.Nil(t, err)
	assert.Equal(t, statusCode, 400)
	assert.Equal(t, string(responseBody), "Bad Request")
}
