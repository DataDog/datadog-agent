// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/pkg/config"
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

	client := defaultforwarder.NewHTTPClient(config.Datadog())

	// With the correct API Key, it should be a 200
	statusCodeWithKey, responseBodyWithKey, _, errWithKey := sendHTTPRequestToEndpoint(context.Background(), client, ts1.URL, endpointInfoTest, apiKey1)
	assert.NoError(t, errWithKey)
	assert.Equal(t, statusCodeWithKey, 200)
	assert.Equal(t, string(responseBodyWithKey), "OK")

	// With the wrong API Key, it should be a 400
	statusCode, responseBody, _, err := sendHTTPRequestToEndpoint(context.Background(), client, ts1.URL, endpointInfoTest, apiKey2)
	assert.NoError(t, err)
	assert.Equal(t, statusCode, 400)
	assert.Equal(t, string(responseBody), "Bad Request")
}

func TestAcceptRedirection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//  * the original flare request URL, which redirects on HEAD to /post-target
		if r.Method == "HEAD" && r.RequestURI == "/support/flare" {
			// redirect to /post-target.
			w.WriteHeader(307)
		} else {
			w.WriteHeader(500)
			io.WriteString(w, "path not recognized by httptest server")
		}
	}))
	defer ts.Close()

	ddURL := ts.URL

	client := clientWithOneRedirects()

	url := ddURL + "/support/flare"
	statusCode, err := sendHTTPHEADRequestToEndpoint(url, client)
	assert.Equal(t, 307, statusCode)
	assert.NoError(t, err)

	url2 := ddURL + "/flare/support"
	statusCode2, err2 := sendHTTPHEADRequestToEndpoint(url2, client)
	assert.Equal(t, 500, statusCode2)
	assert.Error(t, err2)

}
