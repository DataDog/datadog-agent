package api

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func assertingServer(t *testing.T, onReq func(req *http.Request, reqBody []byte) error) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := ioutil.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.NoError(t, onReq(req, body))
		_, err = w.Write([]byte("OK"))
		assert.NoError(t, err)
		req.Body.Close()
	}))
}

func TestTelemetryBasicProxyRequest(t *testing.T) {
	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal(t, "body", string(body), "invalid request body")
		assert.Equal(t, "test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal(t, "test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal(t, "test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal(t, "/path", req.URL.Path)

		return nil
	})
	defer mockConfig("apm_config.telemetry.dd_url", srv.URL)() // reset config after the test

	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	conf := &traceconfig.AgentConfig{
		Hostname:   "test_hostname",
		DefaultEnv: "test_env",
		Endpoints: []*traceconfig.Endpoint{
			{
				APIKey: "test_apikey",
			},
		},
	}
	recv := NewHTTPReceiver(conf, nil, nil, nil)
	recv.buildMux().ServeHTTP(rec, req)
	responseBody, err := ioutil.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(responseBody) != "OK" {
		t.Fatalf("proxy failed %s", responseBody)
	}
}

func TestTelemetryProxyMultipleEndpoitns(t *testing.T) {
	var endpointCalled uint64

	mainBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal(t, "body", string(body), "invalid request body")
		assert.Equal(t, "test_apikey_1", req.Header.Get("DD-API-KEY"))
		assert.Equal(t, "test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal(t, "test_env", req.Header.Get("DD-Agent-Env"))

		atomic.AddUint64(&endpointCalled, 2)
		return nil
	})
	additionalBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal(t, "body", string(body), "invalid request body")
		assert.Equal(t, "test_apikey_2", req.Header.Get("DD-API-KEY"))
		assert.Equal(t, "test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal(t, "test_env", req.Header.Get("DD-Agent-Env"))

		atomic.AddUint64(&endpointCalled, 3)
		return nil
	})

	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	conf := &traceconfig.AgentConfig{
		Hostname:   "test_hostname",
		DefaultEnv: "test_env",
		Endpoints: []*traceconfig.Endpoint{
			{
				APIKey: "test_apikey_1",
			},
		},
	}

	additionalEndpoints := make(map[string]string)
	additionalEndpoints[additionalBackend.URL+"/"] = "test_apikey_2"

	defer mockConfigMap(map[string]interface{}{
		"apm_config.telemetry.additional_endpoints": additionalEndpoints,
		"apm_config.telemetry.dd_url":               mainBackend.URL})()

	recv := NewHTTPReceiver(conf, nil, nil, nil)
	recv.buildMux().ServeHTTP(rec, req)

	responseBody, err := ioutil.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(responseBody) != "OK" {
		t.Fatalf("proxy failed %s", responseBody)
	}

	// because we use number 2,3 both endpoints must be called to produce 5
	// just counting number of requests could give false results if first endpoint
	// was called twice
	if atomic.LoadUint64(&endpointCalled) != 5 {
		t.Fatalf("calling multiple backends failed")
	}
}
