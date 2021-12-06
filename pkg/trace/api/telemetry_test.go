package api

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func assertingServer(t *testing.T, onReq func(req *http.Request, reqBody []byte) error) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
	defer mockConfigMap(map[string]interface{}{
		"api_key":                     "test_apikey",
		"apm_config.telemetry.dd_url": srv.URL,
		"hostname":                    "test_hostname",
		"skip_ssl_validation":         true,
		"env":                         "test_env",
	})() // reset config after the test

	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	conf, err := traceconfig.Load("/does/not/exists.yaml")
	assert.NoError(t, err)

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

func TestTelemetryProxyMultipleEndpoints(t *testing.T) {
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

	additionalEndpoints := make(map[string]string)
	additionalEndpoints[additionalBackend.URL+"/"] = "test_apikey_2"

	defer mockConfigMap(map[string]interface{}{
		"apm_config.telemetry.additional_endpoints": additionalEndpoints,
		"apm_config.telemetry.dd_url":               mainBackend.URL,
		"api_key":                                   "test_apikey_1",
		"hostname":                                  "test_hostname",
		"skip_ssl_validation":                       true,
		"env":                                       "test_env",
	})()

	conf, err := traceconfig.Load("/does/not/exists.yaml")
	assert.NoError(t, err)
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

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestBasicReverseProxy(t *testing.T) {
	t.Run("Sets correct headers", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/example", nil)
		if err != nil {
			t.Fatal(err)
		}

		// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
		rr := httptest.NewRecorder()

		rp := NewReverseProxy(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "", req.Header.Get("User-Agent"))
			assert.Regexp(t, regexp.MustCompile("trace-agent.*"), req.Header.Get("Via"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}), log.Default())

		rp.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
