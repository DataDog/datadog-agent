// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOpenLineageProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		slurp, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if body := string(slurp); body != "body" {
			t.Fatalf("invalid request body: %q", body)
		}
		if v := req.Header.Get("Authorization"); v != "Bearer 123" {
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
	c := &config.AgentConfig{ContainerIDFromOriginInfo: config.NoopContainerIDFromOriginInfoFunc}
	newOpenLineageProxy(c, []*url.URL{u}, []string{"123"}, "key:val", &statsd.NoOpClient{}).ServeHTTP(rec, req)
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

func TestOpenLineageEndpoint(t *testing.T) {
	t.Run("multiple-endpoints", func(t *testing.T) {
		var cfg config.AgentConfig
		cfg.OpenLineageProxy.DDURL = "us3.datadoghq.com"
		cfg.OpenLineageProxy.APIKey = "test_api_key"
		cfg.OpenLineageProxy.AdditionalEndpoints = map[string][]string{
			"us5.datadoghq.com":       {"test_api_key_2"},
			"datadoghq.eu":            {"test_api_key_3"},
			"datad0g.com":             {"test_api_key_4"},
			"ddstaging.datadoghq.com": {"test_api_key_5"},
		}

		expectedURLs := map[string]bool{
			"https://data-obs-intake.us3.datadoghq.com/api/v1/lineage":       false,
			"https://data-obs-intake.us5.datadoghq.com/api/v1/lineage":       false,
			"https://data-obs-intake.datadoghq.eu/api/v1/lineage":            false,
			"https://data-obs-intake.datad0g.com/api/v1/lineage":             false,
			"https://data-obs-intake.ddstaging.datadoghq.com/api/v1/lineage": false,
		}

		expectedKeys := map[string]bool{
			"test_api_key":   false,
			"test_api_key_2": false,
			"test_api_key_3": false,
			"test_api_key_4": false,
			"test_api_key_5": false,
		}

		urls, keys, err := openLineageEndpoints(&cfg)
		assert.NoError(t, err)
		assert.Equal(t, len(urls), 5)
		assert.Equal(t, len(keys), 5)

		for _, url := range urls {
			urlStr := url.String()
			if _, exists := expectedURLs[urlStr]; exists {
				expectedURLs[urlStr] = true
			} else {
				t.Errorf("Unexpected URL found: %s", urlStr)
			}
		}

		for _, key := range keys {
			if _, exists := expectedKeys[key]; exists {
				expectedKeys[key] = true
			} else {
				t.Errorf("Unexpected key found: %s", key)
			}
		}

		for url, found := range expectedURLs {
			assert.True(t, found, "Expected URL not found: %s", url)
		}

		for key, found := range expectedKeys {
			assert.True(t, found, "Expected key not found: %s", key)
		}
	})

	t.Run("dd-site-fallback", func(t *testing.T) {
		var cfg config.AgentConfig
		cfg.Site = "datadoghq.eu"
		cfg.OpenLineageProxy.APIKey = "test_api_key"

		urls, keys, err := openLineageEndpoints(&cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls[0].String(), "https://data-obs-intake.datadoghq.eu/api/v1/lineage")
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("datadoghq.com", func(t *testing.T) {
		var cfg config.AgentConfig
		cfg.OpenLineageProxy.DDURL = "datadoghq.com"
		cfg.OpenLineageProxy.APIKey = "test_api_key"
		cfg.OpenLineageProxy.AdditionalEndpoints = map[string][]string{}

		urls, keys, err := openLineageEndpoints(&cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls[0].String(), "https://data-obs-intake.datadoghq.com/api/v1/lineage")
		assert.Equal(t, keys, []string{"test_api_key"})

	})

	t.Run("empty-ddurl", func(t *testing.T) {
		var cfg config.AgentConfig
		cfg.OpenLineageProxy.DDURL = ""
		cfg.Site = "datadoghq.com"
		cfg.OpenLineageProxy.APIKey = "test_api_key"
		urls, _, err := openLineageEndpoints(&cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls[0].String(), "https://data-obs-intake.datadoghq.com/api/v1/lineage")
	})

	t.Run("different-api", func(t *testing.T) {
		var cfg config.AgentConfig
		cfg.OpenLineageProxy.DDURL = "https://intake.testing.com/different-api"
		cfg.Site = "datadoghq.com"
		cfg.OpenLineageProxy.APIKey = "test_api_key"
		urls, _, err := openLineageEndpoints(&cfg)
		assert.NoError(t, err)
		assert.Equal(t, urls[0].String(), "https://intake.testing.com/different-api")
	})
}

func TestOpenLineageProxyHandler(t *testing.T) {
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
		conf.OpenLineageProxy.DDURL = srv.URL
		fmt.Println("srv url", srv.URL)
		receiver := newTestReceiverFromConfig(conf)
		receiver.openLineageProxyHandler().ServeHTTP(httptest.NewRecorder(), req)
		if !called {
			t.Fatal("request not proxied")
		}
	})

	t.Run("proxy_code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		conf := config.New()
		conf.OpenLineageProxy.DDURL = srv.URL
		conf.Endpoints[0].APIKey = "test_api_key"
		conf.OpenLineageProxy.Enabled = true

		req, _ := http.NewRequest("POST", "/some/path", nil)
		receiver := newTestReceiverFromConfig(conf)
		rec := httptest.NewRecorder()
		receiver.openLineageProxyHandler().ServeHTTP(rec, req)
		resp := rec.Result()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("error", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/some/path", nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		conf := newTestReceiverConfig()
		conf.OpenLineageProxy.DDURL = "asd:\r\n"
		r := newTestReceiverFromConfig(conf)
		r.openLineageProxyHandler().ServeHTTP(rec, req)
		res := rec.Result()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("invalid response: %s", res.Status)
		}
		res.Body.Close()
		slurp, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(slurp), "OpenLineage forwarder is OFF") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})
}
