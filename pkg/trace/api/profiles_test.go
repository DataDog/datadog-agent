// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func makeURLs(t *testing.T, ss ...string) []*url.URL {
	var urls []*url.URL
	for _, s := range ss {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatalf("Cannot parse url: %s", s)
		}
		urls = append(urls, u)
	}
	return urls
}

func TestProfileProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
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
	c := &config.AgentConfig{}
	newProfileProxy(c, []*url.URL{u}, []string{"123"}, "key:val", &statsd.NoOpClient{}).ServeHTTP(rec, req)
	result := rec.Result()
	slurp, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(slurp) != "OK" {
		t.Fatal("did not proxy")
	}
}

func TestProfilingEndpoints(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: "https://intake.profile.datadoghq.fr/api/v2/profile",
		}
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.fr/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("site", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		cfg.Site = "datadoghq.eu"
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.eu/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("default", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "test_api_key"
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.com/api/v2/profile"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("multiple", func(t *testing.T) {
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: "https://intake.profile.datadoghq.jp/api/v2/profile",
			AdditionalEndpoints: map[string][]string{
				"https://ddstaging.datadoghq.com": {"api_key_1", "api_key_2"},
				"https://dd.datad0g.com":          {"api_key_3"},
			},
		}
		urls, keys, err := profilingEndpoints(cfg)
		assert.NoError(t, err)
		expectedURLs := makeURLs(t,
			"https://intake.profile.datadoghq.jp/api/v2/profile",
			"https://ddstaging.datadoghq.com",
			"https://ddstaging.datadoghq.com",
			"https://dd.datad0g.com",
		)
		expectedKeys := []string{"api_key_0", "api_key_1", "api_key_2", "api_key_3"}

		// Because we're using a map to mock the config we can't assert on the
		// order of the endpoints. We check the main endpoints separately.
		assert.Equal(t, urls[0], expectedURLs[0], "The main endpoint should be the first in the slice")
		assert.Equal(t, keys[0], expectedKeys[0], "The main api key should be the first in the slice")

		assert.ElementsMatch(t, urls, expectedURLs, "All urls from the config should be returned")
		assert.ElementsMatch(t, keys, keys, "All keys from the config should be returned")

		// check that we have the correct pairing between urls and api keys
		for i := range keys {
			for j := range expectedKeys {
				if keys[i] == expectedKeys[j] {
					assert.Equal(t, urls[i], expectedURLs[j])
				}
			}
		}
	})
}

func TestProfileProxyHandler(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				if strings.Contains(kv[0], "orchestrator") {
					t.Fatalf("non-fargate environment shouldn't contain '%s' tag : %q", kv[0], v)
				}
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"host", "default_env", "agent_version"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("proxy_code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		req, _ := http.NewRequest("POST", "/some/path", nil)
		receiver := newTestReceiverFromConfig(conf)
		rec := httptest.NewRecorder()
		receiver.profileProxyHandler().ServeHTTP(rec, req)
		resp := rec.Result()
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("ok_fargate", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "orchestrator:fargate_orchestrator") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.Hostname = "myhost"
		conf.FargateOrchestrator = "orchestrator"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("error", func(t *testing.T) {
		conf := newTestReceiverConfig()
		conf.Site = "asd:\r\n"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		r := newTestReceiverFromConfig(conf)
		r.profileProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		slurp, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "error parsing main profiling intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(_ http.ResponseWriter, req *http.Request) {
			called[fmt.Sprintf("http://%s|%s", req.Host, req.Header.Get("DD-API-KEY"))] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))
		cfg := config.New()
		cfg.Endpoints[0].APIKey = "api_key_0"
		cfg.Hostname = "myhost"
		cfg.ProfilingProxy = config.ProfilingProxyConfig{
			DDURL: srv1.URL,
			AdditionalEndpoints: map[string][]string{
				srv2.URL: {"dummy_api_key_1", "dummy_api_key_2"},
				// this should be ignored
				"foobar": {"invalid_url"},
			},
		}

		req, err := http.NewRequest("POST", "/some/path", bytes.NewBuffer([]byte("abc")))
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(cfg)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)

		expected := map[string]bool{
			srv1.URL + "|api_key_0":       true,
			srv2.URL + "|dummy_api_key_1": true,
			srv2.URL + "|dummy_api_key_2": true,
		}
		assert.Equal(t, expected, called, "The request should be proxied to all valid targets")
	})

	t.Run("lambda_function", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			if !strings.Contains(v, "functionname:my-function-name") || !strings.Contains(v, "_dd.origin:lambda") {
				t.Fatalf("invalid X-Datadog-Additional-Tags header, fargate env should contain '%s' tag: %q", "orchestrator", v)
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.LambdaFunctionName = "my-function-name"
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("azure_container_app", func(t *testing.T) {
		var called bool
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			v := req.Header.Get("X-Datadog-Additional-Tags")
			tags := strings.Split(v, ",")
			m := make(map[string]string)
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				m[kv[0]] = kv[1]
			}
			for _, tag := range []string{"subscription_id", "resource_group", "resource_id"} {
				if _, ok := m[tag]; !ok {
					t.Fatalf("invalid X-Datadog-Additional-Tags header, should contain '%s': %q", tag, v)
				}
			}
			called = true
		}))
		conf := newTestReceiverConfig()
		conf.ProfilingProxy = config.ProfilingProxyConfig{DDURL: srv.URL}
		conf.GlobalTags = map[string]string{
			"subscription_id": "123",
			"resource_group":  "test-rg",
			"resource_id":     "456",
		}
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})
}
