// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasValidAPIKey(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()
	defer ts2.Close()

	keysPerDomains := map[string][]string{
		ts1.URL: {"api_key1", "api_key2"},
		ts2.URL: {"key3"},
	}

	fh := newForwarderHealth(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey())

	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with key3"))
}
func TestComputeAPIDomains(t *testing.T) {
	keysPerDomains := map[string][]string{
		"https://app.datadoghq.com":    {"api_key_1", "api_key_1b"},
		"https://app.datadoghq.eu":     {"api_key_2"},
		"https://app.datad0g.com":      {"api_key_3", "api_key_3b"},
		"https://app.datad0g.eu":       {"api_key_4"},
		"https://custom.datadoghq.com": {"api_key_5"},
		"https://nochange.com":         {"api_key_6", "api_key_6b"},
		":datadoghq.com":               {"api_key_7"},
	}

	testMap := map[string][]string{
		"https://api.datadoghq.com":    {"api_key_1", "api_key_1b"},
		"https://api.datadoghq.eu":     {"api_key_2"},
		"https://api.datad0g.com":      {"api_key_3", "api_key_3b"},
		"https://api.datad0g.eu":       {"api_key_4"},
		"https://custom.datadoghq.com": {"api_key_5"},
		"https://nochange.com":         {"api_key_6", "api_key_6b"},
	}

	fh := newForwarderHealth(keysPerDomains)

	assert.Equal(t, fh.keysPerAPIDomain, testMap)
}

func TestHasValidAPIKeyErrors(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("api_key") == "api_key1" {
			w.WriteHeader(http.StatusForbidden)
		} else if r.Form.Get("api_key") == "api_key2" {
			w.WriteHeader(http.StatusNotFound)
		} else {
			assert.Fail(t, fmt.Sprintf("Unknown api key received: %v", r.Form.Get("api_key")))
		}
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()
	defer ts2.Close()

	keysPerDomains := map[string][]string{
		ts1.URL: {"api_key1", "api_key2"},
		ts2.URL: {"key3"},
	}

	fh := newForwarderHealth(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey())

	assert.Equal(t, &apiKeyInvalid, apiKeyStatus.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyStatusUnknown, apiKeyStatus.Get("API key ending with _key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with key3"))
}
