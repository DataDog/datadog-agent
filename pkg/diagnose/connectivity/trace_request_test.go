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

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
)

var (
	apiKey1 = "api_key1"
	apiKey2 = "api_key2"

	endpointInfoTest = endpointInfo{Endpoint: endpoints.V1ValidateEndpoint}
)

func TestCreateEndpointUrl(t *testing.T) {

	url := createEndpointURL("https://domain", endpointInfoTest)
	assert.Equal(t, url, "https://domain/api/v1/validate")
}

func TestSendHTTPRequestToEndpoint(t *testing.T) {

	// Create a fake server that send a 200 Response if there the 'DD-API-KEY' header has value 'api_key1'
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Header.Get("DD-API-KEY") == "api_key1" {
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Bad Request"))
		}
	}))
	defer ts1.Close()

	client := forwarder.NewHTTPClient()

	// With the correct API Key, it should be a 200
	statusCodeWithKey, responseBodyWithKey, errWithKey := sendHTTPRequestToEndpoint(context.Background(), color.Output, client, ts1.URL, endpointInfoTest, apiKey1)
	assert.Nil(t, errWithKey)
	assert.Equal(t, statusCodeWithKey, 200)
	assert.Equal(t, string(responseBodyWithKey), "OK")

	// With the wrong API Key, it should be a 400
	statusCode, responseBody, err := sendHTTPRequestToEndpoint(context.Background(), color.Output, client, ts1.URL, endpointInfoTest, apiKey2)
	assert.Nil(t, err)
	assert.Equal(t, statusCode, 400)
	assert.Equal(t, string(responseBody), "Bad Request")
}
