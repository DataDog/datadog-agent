// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
		ts2.URL: {"api_key3"},
	}

	fh := forwarderHealth{}
	fh.init(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey(keysPerDomains))

	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get(ts1.URL+",*************************_key1"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get(ts1.URL+",*************************_key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get(ts2.URL+",*************************_key3"))
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
		ts2.URL: {"api_key3"},
	}

	fh := forwarderHealth{}
	fh.init(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey(keysPerDomains))

	assert.Equal(t, &apiKeyInvalid, apiKeyStatus.Get(ts1.URL+",*************************_key1"))
	assert.Equal(t, &apiKeyStatusUnknown, apiKeyStatus.Get(ts1.URL+",*************************_key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get(ts2.URL+",*************************_key3"))
}
