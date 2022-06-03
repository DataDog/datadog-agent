// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripperMock func(*http.Request) (*http.Response, error)

func (r roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// sendRequestThroughHandler sends a request through the evpIntakeReverseProxy handler and returns the forwarded request(s), their response and the log output.
// The path for inReq shouldn't have the /evpIntakeProxy/v1 prefix since it is passed directly to the inner proxy handler and not the trace-agent API handler.
func sendRequestThroughHandler(conf *config.AgentConfig, inReq *http.Request) (outReqs []*http.Request, response *http.Response, loggerOut string) {
	mockRoundTripper := roundTripperMock(func(req *http.Request) (*http.Response, error) {
		outReqs = append(outReqs, req)
		// If we got here it means the proxy didn't raise an error earlier, return an ok response
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer([]byte("ok_responserino"))),
		}, nil
	})
	var loggerBuffer bytes.Buffer
	mockLogger := log.New(io.Writer(&loggerBuffer), "", 0)
	handler := evpIntakeReverseProxyHandler(conf, evpIntakeEndpointsFromConfig(conf), mockRoundTripper, mockLogger)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, inReq)
	return outReqs, rec.Result(), loggerBuffer.String()
}

func TestEvpIntakeReverseProxyHandlerOk(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Hostname = "test_hostname"
	conf.DefaultEnv = "test_env"
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	request := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	request.Header.Set("User-Agent", "test_user_agent")
	request.Header.Set("Content-Type", "text/json")
	request.Header.Set("Unexpected-Header", "To-Be-Discarded")
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, http.StatusOK, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Equal(t, 1, len(proxiedRequests))
	proxiedRequest := proxiedRequests[0]
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxiedRequest.Host)
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxiedRequest.URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxiedRequest.URL.Path)
	assert.Equal(t, "arg=test", proxiedRequest.URL.RawQuery)
	assert.Equal(t, "test_api_key", proxiedRequest.Header.Get("DD-API-KEY"))
	assert.Equal(t, conf.Hostname, proxiedRequest.Header.Get("X-Datadog-Hostname"))
	assert.Equal(t, conf.DefaultEnv, proxiedRequest.Header.Get("X-Datadog-AgentDefaultEnv"))
	assert.Equal(t, fmt.Sprintf("trace-agent %s", info.Version), proxiedRequest.Header.Get("Via"))
	assert.Equal(t, "test_user_agent", proxiedRequest.Header.Get("User-Agent"))
	assert.Equal(t, "text/json", proxiedRequest.Header.Get("Content-Type"))
	assert.NotContains(t, proxiedRequest.Header, "Unexpected-Header")
	assert.NotContains(t, proxiedRequest.Header, "X-Datadog-Container-Tags")
	assert.Equal(t, "", loggerOut)
}

func TestEvpIntakeReverseProxyHandlerWithContainerId(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.ContainerTags = func(cid string) ([]string, error) {
		return []string{"container:" + cid}, nil
	}

	request := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	request.Header.Set("Datadog-Container-ID", "myid")
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, http.StatusOK, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Equal(t, 1, len(proxiedRequests))
	assert.Equal(t, "container:myid", proxiedRequests[0].Header.Get("X-Datadog-Container-Tags"))
	assert.Equal(t, "", loggerOut)
}

func TestEvpIntakeReverseProxyHandlerMultipleEndpoints(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.EvpIntakeProxy.AdditionalEndpoints = map[string][]string{
		"datadoghq.com": []string{"test_api_key_1", "test_api_key_2"},
		"datadoghq.eu":  []string{"test_api_key_eu"},
	}
	request := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	request.Header.Set("X-Datadog-Agent", "test_user_agent")
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, http.StatusOK, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Equal(t, 4, len(proxiedRequests))

	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxiedRequests[0].Host)
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxiedRequests[0].URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxiedRequests[0].URL.Path)
	assert.Equal(t, "arg=test", proxiedRequests[0].URL.RawQuery)
	assert.Equal(t, "test_api_key", proxiedRequests[0].Header.Get("DD-API-KEY"))

	assert.Equal(t, "mysubdomain.datadoghq.com", proxiedRequests[1].Host)
	assert.Equal(t, "mysubdomain.datadoghq.com", proxiedRequests[1].URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxiedRequests[1].URL.Path)
	assert.Equal(t, "arg=test", proxiedRequests[1].URL.RawQuery)
	assert.Equal(t, "test_api_key_1", proxiedRequests[1].Header.Get("DD-API-KEY"))

	assert.Equal(t, "mysubdomain.datadoghq.com", proxiedRequests[2].Host)
	assert.Equal(t, "mysubdomain.datadoghq.com", proxiedRequests[2].URL.Host)
	assert.Equal(t, "test_api_key_2", proxiedRequests[2].Header.Get("DD-API-KEY"))

	assert.Equal(t, "mysubdomain.datadoghq.eu", proxiedRequests[3].Host)
	assert.Equal(t, "mysubdomain.datadoghq.eu", proxiedRequests[3].URL.Host)
	assert.Equal(t, "test_api_key_eu", proxiedRequests[3].Header.Get("DD-API-KEY"))

	assert.Equal(t, "", loggerOut)
}

func TestEvpIntakeReverseProxyHandlerInvalidSubdomain(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	request := httptest.NewRequest("POST", "/google.com%3Fattack=/mypath/mysubpath", nil)
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, 0, len(proxiedRequests))
	require.Equal(t, http.StatusBadGateway, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Contains(t, loggerOut, "invalid subdomain")
}

func TestEvpIntakeReverseProxyHandlerInvalidPath(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	request := httptest.NewRequest("POST", "/mysubdomain/mypath/my%20subpath", nil)
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, 0, len(proxiedRequests))
	require.Equal(t, http.StatusBadGateway, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Contains(t, loggerOut, "invalid target path")
}

func TestEvpIntakeReverseProxyHandlerInvalidQuery(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	request := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?test=bad%20arg", nil)
	proxiedRequests, response, loggerOut := sendRequestThroughHandler(conf, request)

	require.Equal(t, 0, len(proxiedRequests))
	require.Equal(t, http.StatusBadGateway, response.StatusCode, "Got: ", fmt.Sprint(response.StatusCode))
	require.Contains(t, loggerOut, "invalid query string")
}
