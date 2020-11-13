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

	"github.com/stretchr/testify/assert"

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

func printEndpoints(endpoints []*traceconfig.Endpoint) []string {
	ss := []string{}
	for _, e := range endpoints {
		ss = append(ss, e.Host+"|"+e.APIKey)
	}
	return ss
}

func TestProfilingEndpoints(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		defer mockConfig("apm_config.profiling_dd_url", "https://intake.profile.datadoghq.fr/v1/input")()
		urls, keys, err := profilingEndpoints("test_api_key")
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.fr/v1/input"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})
	t.Run("site", func(t *testing.T) {
		defer mockConfig("site", "datadoghq.eu")()
		urls, keys, err := profilingEndpoints("test_api_key")
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.eu/v1/input"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("default", func(t *testing.T) {
		urls, keys, err := profilingEndpoints("test_api_key")
		assert.NoError(t, err)
		assert.Equal(t, urls, makeURLs(t, "https://intake.profile.datadoghq.com/v1/input"))
		assert.Equal(t, keys, []string{"test_api_key"})
	})

	t.Run("multiple", func(t *testing.T) {
		defer mockConfigMap(map[string]interface{}{
			"apm_config.profiling_dd_url": "https://intake.profile.datadoghq.jp/v1/input",
			"apm_config.profiling_additional_endpoints": map[string][]string{
				"https://ddstaging.datadoghq.com": {"api_key_1", "api_key_2"},
				"https://dd.datad0g.com":          {"api_key_3"},
			},
		})()
		urls, keys, err := profilingEndpoints("api_key_0")
		assert.NoError(t, err)
		expectedURLs := makeURLs(t,
			"https://intake.profile.datadoghq.jp/v1/input",
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
		if !strings.Contains(string(slurp), "error parsing main profiling intake URL") {
			t.Fatalf("invalid message: %q", slurp)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		called := make(map[string]bool)
		handler := func(w http.ResponseWriter, req *http.Request) {
			called[fmt.Sprintf("http://%s|%s", req.Host, req.Header.Get("DD-API-KEY"))] = true
		}
		srv1 := httptest.NewServer(http.HandlerFunc(handler))
		srv2 := httptest.NewServer(http.HandlerFunc(handler))
		defer mockConfigMap(map[string]interface{}{
			"apm_config.profiling_dd_url": srv1.URL,
			"apm_config.profiling_additional_endpoints": map[string][]string{
				srv2.URL: {"dummy_api_key_1", "dummy_api_key_2"},
				// this should be ignored
				"foobar": {"invalid_url"},
			},
		})()

		req, err := http.NewRequest("POST", "/some/path", bytes.NewBuffer([]byte("abc")))
		if err != nil {
			t.Fatal(err)
		}
		conf := newTestReceiverConfig()
		conf.Hostname = "myhost"
		receiver := newTestReceiverFromConfig(conf)
		receiver.profileProxyHandler().ServeHTTP(httptest.NewRecorder(), req)

		expected := map[string]bool{
			srv1.URL + "|test":            true,
			srv2.URL + "|dummy_api_key_1": true,
			srv2.URL + "|dummy_api_key_2": true,
		}
		assert.Equal(t, expected, called, "The request should be proxied to all valid targets")
	})
}
