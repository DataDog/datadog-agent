// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/stretchr/testify/assert"
)

var (
	apiKey                    = "api_key1"
	endpointInfoWithApiKey    = EndpointInfo{Endpoint: endpoints.V1ValidateEndpoint, APIKeyInQueryString: true}
	endpointInfoWithoutApiKey = EndpointInfo{Endpoint: endpoints.V1SeriesEndpoint, APIKeyInQueryString: false}
)

func TestCreateEndpointUrl(t *testing.T) {

	urlWithApiKey := createEndpointURL("https://domain", endpointInfoWithApiKey, apiKey)
	urlWithoutApiKey := createEndpointURL("https://domain2", endpointInfoWithoutApiKey, apiKey)

	assert.Equal(t, urlWithApiKey, "https://domain/api/v1/validate?api_key=api_key1")
	assert.Equal(t, urlWithoutApiKey, "https://domain2/api/v1/series")
}

func TestSendHTTPRequestToEndpointPass(t *testing.T) {

	// Create a fake server that send a 200 Response if there is 'api_key1' in the query string
	// or a 400 response otherwise.
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("api_key") == "api_key1" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts1.Close()

	client := forwarder.NewHTTPClient()

	output := captureOutputSendHTTPRequestToEndpoint(client, ts1.URL, endpointInfoWithApiKey, apiKey)
	assert.True(t, strings.Contains(output, "PASS"))

	output2 := captureOutputSendHTTPRequestToEndpoint(client, ts1.URL, endpointInfoWithoutApiKey, apiKey)
	assert.True(t, strings.Contains(output2, "FAIL"))
}

// captureOutputSendHTTPRequestToEndpoint is a helper to get the output of the
// sendHTTPRequestToEndpoint function into a string
func captureOutputSendHTTPRequestToEndpoint(client *http.Client, domain string, endpointInfo EndpointInfo, apiKey string) string {
	rescueStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	sendHTTPRequestToEndpoint(client, domain, endpointInfo, apiKey)

	w.Close()
	out, _ := ioutil.ReadAll(r)
	os.Stdout = rescueStdout

	return string(out)
}
