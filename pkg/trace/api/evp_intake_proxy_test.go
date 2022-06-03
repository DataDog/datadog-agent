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

// sendRequestThroughForwarder sends a request through the evpProxyForwarder handler and returns the forwarded
// request(s), their response and the log output. The path for inReq shouldn't have the /evp_proxy/v1/input
// prefix since it is passed directly to the inner proxy handler and not the trace-agent API handler.
func sendRequestThroughForwarder(conf *config.AgentConfig, inReq *http.Request) (outReqs []*http.Request, resp *http.Response, logs string) {
	mockRoundTripper := roundTripperMock(func(req *http.Request) (*http.Response, error) {
		outReqs = append(outReqs, req)
		// If we got here it means the proxy didn't raise an error earlier, return an ok resp
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer([]byte("ok_resprino"))),
		}, nil
	})
	var loggerBuffer bytes.Buffer
	mockLogger := log.New(io.Writer(&loggerBuffer), "", 0)
	handler := evpProxyForwarder(conf, evpProxyEndpointsFromConfig(conf), mockRoundTripper, mockLogger)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, inReq)
	return outReqs, rec.Result(), loggerBuffer.String()
}

func TestEVPProxyForwarderOk(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Hostname = "test_hostname"
	conf.DefaultEnv = "test_env"
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	req := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	req.Header.Set("User-Agent", "test_user_agent")
	req.Header.Set("Content-Type", "text/json")
	req.Header.Set("Unexpected-Header", "To-Be-Discarded")
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Equal(t, 1, len(proxyreqs))
	proxyreq := proxyreqs[0]
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxyreq.Host)
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxyreq.URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxyreq.URL.Path)
	assert.Equal(t, "arg=test", proxyreq.URL.RawQuery)
	assert.Equal(t, "test_api_key", proxyreq.Header.Get("DD-API-KEY"))
	assert.Equal(t, conf.Hostname, proxyreq.Header.Get("X-Datadog-Hostname"))
	assert.Equal(t, conf.DefaultEnv, proxyreq.Header.Get("X-Datadog-AgentDefaultEnv"))
	assert.Equal(t, fmt.Sprintf("trace-agent %s", info.Version), proxyreq.Header.Get("Via"))
	assert.Equal(t, "test_user_agent", proxyreq.Header.Get("User-Agent"))
	assert.Equal(t, "text/json", proxyreq.Header.Get("Content-Type"))
	assert.NotContains(t, proxyreq.Header, "Unexpected-Header")
	assert.NotContains(t, proxyreq.Header, "X-Datadog-Container-Tags")
	assert.Equal(t, "", logs)
}

func TestEVPProxyForwarderWithContainerId(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.ContainerTags = func(cid string) ([]string, error) {
		return []string{"container:" + cid}, nil
	}

	req := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	req.Header.Set("Datadog-Container-ID", "myid")
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Equal(t, 1, len(proxyreqs))
	assert.Equal(t, "container:myid", proxyreqs[0].Header.Get("X-Datadog-Container-Tags"))
	assert.Equal(t, "", logs)
}

func TestEVPProxyForwarderMultipleEndpoints(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.EVPProxy.AdditionalEndpoints = map[string][]string{
		"datadoghq.eu": []string{"test_api_key_1", "test_api_key_2"},
	}
	req := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?arg=test", nil)
	req.Header.Set("X-Datadog-Agent", "test_user_agent")
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Equal(t, 3, len(proxyreqs))

	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxyreqs[0].Host)
	assert.Equal(t, "mysubdomain.us3.datadoghq.com", proxyreqs[0].URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxyreqs[0].URL.Path)
	assert.Equal(t, "arg=test", proxyreqs[0].URL.RawQuery)
	assert.Equal(t, "test_api_key", proxyreqs[0].Header.Get("DD-API-KEY"))

	assert.Equal(t, "mysubdomain.datadoghq.eu", proxyreqs[1].Host)
	assert.Equal(t, "mysubdomain.datadoghq.eu", proxyreqs[1].URL.Host)
	assert.Equal(t, "/mypath/mysubpath", proxyreqs[1].URL.Path)
	assert.Equal(t, "arg=test", proxyreqs[1].URL.RawQuery)
	assert.Equal(t, "test_api_key_1", proxyreqs[1].Header.Get("DD-API-KEY"))

	assert.Equal(t, "mysubdomain.datadoghq.eu", proxyreqs[2].Host)
	assert.Equal(t, "mysubdomain.datadoghq.eu", proxyreqs[2].URL.Host)
	assert.Equal(t, "test_api_key_2", proxyreqs[2].Header.Get("DD-API-KEY"))

	assert.Equal(t, "", logs)
}

func TestEVPProxyForwarderInvalidSubdomain(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	req := httptest.NewRequest("POST", "/google.com%3Fattack=/mypath/mysubpath", nil)
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, 0, len(proxyreqs))
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Contains(t, logs, "invalid subdomain")
}

func TestEVPProxyForwarderInvalidPath(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	req := httptest.NewRequest("POST", "/mysubdomain/mypath/my%20subpath", nil)
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, 0, len(proxyreqs))
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Contains(t, logs, "invalid target path")
}

func TestEVPProxyForwarderInvalidQuery(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"

	req := httptest.NewRequest("POST", "/mysubdomain/mypath/mysubpath?test=bad%20arg", nil)
	proxyreqs, resp, logs := sendRequestThroughForwarder(conf, req)

	require.Equal(t, 0, len(proxyreqs))
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "Got: ", fmt.Sprint(resp.StatusCode))
	require.Contains(t, logs, "invalid query string")
}

func TestEVPProxyEndpointsFromConfigOverride(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.Site = "us3.datadoghq.com"
	conf.Endpoints[0].APIKey = "test_api_key"
	conf.EVPProxy.DDURL = "override.datadoghq.com"
	conf.EVPProxy.APIKey = "override_api_key"

	endpoints := evpProxyEndpointsFromConfig(conf)

	require.Equal(t, 1, len(endpoints))
	require.Equal(t, endpoints[0].Host, "override.datadoghq.com")
	require.Equal(t, endpoints[0].APIKey, "override_api_key")
}

func TestEVPProxyHandler(t *testing.T) {
	cfg := config.New()
	receiver := &HTTPReceiver{conf: cfg}
	handler := receiver.evpProxyHandler()
	require.NotNil(t, handler)
}

func TestEVPProxyHandlerDisabled(t *testing.T) {
	cfg := config.New()
	cfg.EVPProxy.Enabled = false
	receiver := &HTTPReceiver{conf: cfg}
	handler := receiver.evpProxyHandler()
	require.NotNil(t, handler)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/evp_proxy/v1/input/mysubdomain/mypath", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
