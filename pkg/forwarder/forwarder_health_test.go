// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestHasValidAPIKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ddURL := config.Datadog.Get("dd_url")
	config.Datadog.Set("dd_url", ts.URL)
	defer ts.Close()
	defer func() { config.Datadog.Set("dd_url", ddURL) }()

	keysPerDomains := map[string][]string{
		"domain1": {"api_key1", "api_key2"},
		"domain2": {"key3"},
	}

	fh := forwarderHealth{}
	fh.init(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey(keysPerDomains))

	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("domain1,*************************_key1"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("domain1,*************************_key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("domain2,*************************"))
}

func TestHasValidAPIKeyErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("api_key") == "api_key1" {
			w.WriteHeader(http.StatusForbidden)
		} else if r.Form.Get("api_key") == "api_key2" {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	ddURL := config.Datadog.Get("dd_url")
	config.Datadog.Set("dd_url", ts.URL)
	defer ts.Close()
	defer func() { config.Datadog.Set("dd_url", ddURL) }()

	keysPerDomains := map[string][]string{
		"domain1": {"api_key1", "api_key2"},
		"domain2": {"key3"},
	}

	fh := forwarderHealth{}
	fh.init(keysPerDomains)
	assert.True(t, fh.hasValidAPIKey(keysPerDomains))

	assert.Equal(t, &apiKeyInvalid, apiKeyStatus.Get("domain1,*************************_key1"))
	assert.Equal(t, &apiKeyStatusUnknown, apiKeyStatus.Get("domain1,*************************_key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("domain2,*************************"))
}
