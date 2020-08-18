package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
)

func mockConfig(k string, v interface{}) func() {
	oldConfig := config.Datadog
	config.Mock().Set(k, v)
	return func() { config.Datadog = oldConfig }
}

func mockConfigMap(m map[string]interface{}) func() {
	oldConfig := config.Datadog
	mockConfig := config.Mock()
	for k, v := range m {
		mockConfig.Set(k, v)
	}
	return func() { config.Datadog = oldConfig }
}

func TestProfileProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := ioutil.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body := string(slurp); body != "body" {
			t.Fatalf("invalid request body: %q", body)
		}
		if v := req.Header.Get("DD-API-KEY"); v != "123" {
			t.Fatalf("got invalid API key: %q", v)
		}
		if v := req.Header.Get("X-Datadog-Additional-Tags"); v != "key:val" {
			t.Fatalf("got invalid X-Datadog-Additional-Tags: %q", v)
		}
		_, err = w.Write([]byte("OK"))
		if err != nil {
			t.Fatal(err)
		}
	}))
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", "dummy.com/path", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := &traceconfig.AgentConfig{}
	newProfileProxy(c.NewHTTPTransport(), []*url.URL{u}, []string{"123"}, "key:val").ServeHTTP(rec, req)
	slurp, err := ioutil.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestMainProfilingEndpoint(t *testing.T) {
	t.Run("dd_url", func(t *testing.T) {
		defer mockConfig("apm_config.profiling_dd_url", "https://intake.profile.datadoghq.fr/v1/input")()
		if v := mainProfilingEndpoint(); v != "https://intake.profile.datadoghq.fr/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})

	t.Run("site", func(t *testing.T) {
		defer mockConfig("site", "datadoghq.eu")()
		if v := mainProfilingEndpoint(); v != "https://intake.profile.datadoghq.eu/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})

	t.Run("default", func(t *testing.T) {
		if v := mainProfilingEndpoint(); v != "https://intake.profile.datadoghq.com/v1/input" {
			t.Fatalf("invalid endpoint: %s", v)
		}
	})
}

func TestAdditionalProfilingEndpoints(t *testing.T) {
	t.Run("additional endpoints empty", func(t *testing.T) {
		additionalEndpoints := additionalProfilingEndpoints()
		if len(additionalProfilingEndpoints()) != 0 {
			t.Fatalf("additional endpoints should be empty but was %v", additionalEndpoints)
		}
	})

	t.Run("additional endpoints with multiple api keys", func(t *testing.T) {
		endpointConfig := make(map[string][]string)
		endpointConfig["https://ddstaging.datadoghq.com"] = []string{"api_key_1", "api_key_2"}
		endpointConfig["https://dd.datad0g.com"] = []string{"api_key_staging"}
		defer mockConfig("apm_config.profiling_additional_endpoints", endpointConfig)()
		additionalEndpoints := additionalProfilingEndpoints()
		if len(additionalEndpoints) != 2 ||
			len(additionalEndpoints["https://ddstaging.datadoghq.com"]) != 2 ||
			len(additionalEndpoints["https://dd.datad0g.com"]) != 1 {
			t.Fatalf("expecting additional endpoints to be fully populated but was %v", additionalEndpoints)
		}
	})
}

func TestProfileProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if v := req.Header.Get("X-Datadog-Additional-Tags"); v != "host:myhost,default_env:none" {
				t.Fatalf("invalid X-Datadog-Additional-Tags header: %q", v)
			}
			called = true
		}))
		defer mockConfig("apm_config.profiling_dd_url", srv.URL)()
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		defer mockConfig("site", "asd:\r\n")()
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r := newTestReceiverFromConfig(newTestReceiverConfig())
		r.profileProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "invalid intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(w http.ResponseWriter, req *http.Request) {
			called[fmt.Sprintf("%s|%s", req.Host, req.Header.Get("DD-API-KEY"))] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))

		additionalEndpoints := make(map[string][]string)
		additionalEndpoints[srv2.URL] = []string{"dummy_api_key_1", "dummy_api_key_2"}
		// this should be ignored
		additionalEndpoints["foobar"] = []string{"invalid_url"}
		defer mockConfigMap(map[string]interface{}{
			"apm_config.profiling_dd_url":               srv1.URL,
			"apm_config.profiling_additional_endpoints": additionalEndpoints,
		})()

		req, err := http.NewRequest("POST", "/some/path", bytes.NewBuffer([]byte("abc")))
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if len(called) != 3 {
			t.Fatalf("request not proxied to all targets, seen: %v", called)
		}
	})
}
