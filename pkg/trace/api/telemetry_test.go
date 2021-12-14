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

func newRequestRecorder(t *testing.T) (req *http.Request, rec *httptest.ResponseRecorder) {
	req, err := http.NewRequest("POST", "/telemetry/proxy/path", strings.NewReader("body"))
	assert.NoError(t, err)
	rec = httptest.NewRecorder()
	return req, rec
}

func recordedResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	responseBody, err := ioutil.ReadAll(rec.Result().Body)
	assert.NoError(t, err)

	return string(responseBody)
}

func loadConfig(t *testing.T) *traceconfig.AgentConfig {
	cfg, err := traceconfig.Load("/does/not/exists.yaml")
	assert.NoError(t, err)
	return cfg
}

func TestTelemetryBasicProxyRequest(t *testing.T) {
	var endpointCalled uint64
	assert := assert.New(t)

	srv := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))
		assert.Equal("/path", req.URL.Path)

		atomic.AddUint64(&endpointCalled, 1)
		return nil
	})
	defer mockConfigMap(map[string]interface{}{
		"api_key":                     "test_apikey",
		"apm_config.telemetry.dd_url": srv.URL,
		"hostname":                    "test_hostname",
		"skip_ssl_validation":         true,
		"env":                         "test_env",
	})() // reset config after the test

	req, rec := newRequestRecorder(t)
	recv := NewHTTPReceiver(loadConfig(t), nil, nil, nil)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))
	assert.Equal(uint64(1), atomic.LoadUint64(&endpointCalled))
}

func TestTelemetryProxyMultipleEndpoints(t *testing.T) {
	var endpointCalled uint64
	assert := assert.New(t)

	mainBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_1", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))

		atomic.AddUint64(&endpointCalled, 2)
		return nil
	})
	additionalBackend := assertingServer(t, func(req *http.Request, body []byte) error {
		assert.Equal("body", string(body), "invalid request body")
		assert.Equal("test_apikey_2", req.Header.Get("DD-API-KEY"))
		assert.Equal("test_hostname", req.Header.Get("DD-Agent-Hostname"))
		assert.Equal("test_env", req.Header.Get("DD-Agent-Env"))

		atomic.AddUint64(&endpointCalled, 3)
		return nil
	})

	defer mockConfigMap(map[string]interface{}{
		"apm_config.telemetry.additional_endpoints": map[string]string{
			additionalBackend.URL + "/": "test_apikey_2",
			// proxy must ignore malformed urls
			"111://malformed_url.example.com": "test_apikey_3",
		},
		"apm_config.telemetry.dd_url": mainBackend.URL,
		"api_key":                     "test_apikey_1",
		"hostname":                    "test_hostname",
		"skip_ssl_validation":         true,
		"env":                         "test_env",
	})()

	req, rec := newRequestRecorder(t)
	recv := NewHTTPReceiver(loadConfig(t), nil, nil, nil)
	recv.buildMux().ServeHTTP(rec, req)

	assert.Equal("OK", recordedResponse(t, rec))

	// because we use number 2,3 both endpoints must be called to produce 5
	// just counting number of requests could give false results if first endpoint
	// was called twice
	if atomic.LoadUint64(&endpointCalled) != 5 {
		t.Fatalf("calling multiple backends failed")
	}
}

func TestTelemetryConfig(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		defer mockConfigMap(map[string]interface{}{
			"apm_config.telemetry.enabled": false,
			"api_key":                      "api_key",
		})()

		req, rec := newRequestRecorder(t)
		recv := NewHTTPReceiver(loadConfig(t), nil, nil, nil)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Contains(t, recordedResponse(t, rec), "Telemetry proxy forwarder is Disabled")
	})

	t.Run("no-endpoints", func(t *testing.T) {
		defer mockConfigMap(map[string]interface{}{
			"apm_config.telemetry.dd_url": "111://malformed.dd_url.com",
			"api_key":                     "api_key",
		})()

		req, rec := newRequestRecorder(t)
		recv := NewHTTPReceiver(loadConfig(t), nil, nil, nil)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Contains(t, recordedResponse(t, rec), "Telemetry proxy forwarder doesn't have any valid endpoints")
	})

	t.Run("fallback-endpoint", func(t *testing.T) {
		srv := assertingServer(t, func(req *http.Request, body []byte) error { return nil })
		defer mockConfigMap(map[string]interface{}{
			"apm_config.telemetry.dd_url": "111://malformed.dd_url.com",
			"apm_config.telemetry.additional_endpoints": map[string]string{
				srv.URL: "api_key",
			},
			"skip_ssl_validation": true,
			"api_key":             "api_key",
		})()
		req, rec := newRequestRecorder(t)
		recv := NewHTTPReceiver(loadConfig(t), nil, nil, nil)
		recv.buildMux().ServeHTTP(rec, req)

		assert.Equal(t, "OK", recordedResponse(t, rec))
	})
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (s roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return s(r)
}

func TestBasicReverseProxy(t *testing.T) {
	t.Run("headers", func(t *testing.T) {
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
