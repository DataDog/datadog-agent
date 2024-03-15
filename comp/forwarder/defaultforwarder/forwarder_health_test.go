// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCheckValidAPIKey(t *testing.T) {
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
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	cfg := fxutil.Test[config.Component](t, config.MockModule())
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: resolver.NewSingleDomainResolvers(keysPerDomains)}
	fh.init()
	assert.True(t, fh.checkValidAPIKey())

	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with key3"))
}

func TestComputeDomainsURL(t *testing.T) {
	keysPerDomains := map[string][]string{
		"https://app.datadoghq.com":              {"api_key1"},
		"https://custom.datadoghq.com":           {"api_key2"},
		"https://custom.agent.datadoghq.com":     {"api_key3"},
		"https://app.datadoghq.eu":               {"api_key4"},
		"https://app.us2.datadoghq.com":          {"api_key5"},
		"https://app.xx9.datadoghq.com":          {"api_key5"},
		"https://custom.agent.us2.datadoghq.com": {"api_key6"},
		// debatable whether the next one should be changed to `api.`, preserve pre-existing behavior for now
		"https://app.datadoghq.internal": {"api_key7"},
		"https://app.myproxy.com":        {"api_key8"},
		"https://app.ddog-gov.com":       {"api_key9"},
		"https://custom.ddog-gov.com":    {"api_key10"},
	}

	expectedMap := map[string][]string{
		"https://api.datadoghq.com":      {"api_key1", "api_key2", "api_key3"},
		"https://api.datadoghq.eu":       {"api_key4"},
		"https://api.us2.datadoghq.com":  {"api_key5", "api_key6"},
		"https://api.xx9.datadoghq.com":  {"api_key5"},
		"https://api.datadoghq.internal": {"api_key7"},
		"https://app.myproxy.com":        {"api_key8"},
		"https://api.ddog-gov.com":       {"api_key9", "api_key10"},
	}

	// just sort the expected map for easy comparison
	for _, keys := range expectedMap {
		sort.Strings(keys)
	}
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	fh := forwarderHealth{log: log, domainResolvers: resolver.NewSingleDomainResolvers(keysPerDomains)}
	fh.init()

	// lexicographical sort for assert
	for _, keys := range fh.keysPerAPIEndpoint {
		sort.Strings(keys)
	}

	assert.Equal(t, expectedMap, fh.keysPerAPIEndpoint)
}

func TestCheckValidAPIKeyErrors(t *testing.T) {
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

	ts3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	ts3.URL = "unreachable/url"

	defer ts1.Close()
	defer ts2.Close()
	defer ts3.Close()

	keysPerAPIEndpoint := map[string][]string{
		ts1.URL: {"api_key1", "api_key2"},
		ts2.URL: {"key3"},
		ts3.URL: {"key4"},
	}
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	cfg := fxutil.Test[config.Component](t, config.MockModule())
	fh := forwarderHealth{log: log, config: cfg}
	fh.init()
	fh.keysPerAPIEndpoint = keysPerAPIEndpoint
	assert.True(t, fh.checkValidAPIKey())

	assert.Equal(t, nil, apiKeyStatus.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyInvalid, apiKeyFailure.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyUnexpectedStatusCode, apiKeyStatus.Get("API key ending with _key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with key3"))
	assert.Equal(t, &apiKeyEndpointUnreachable, apiKeyStatus.Get("API key ending with key4"))
}

func TestUpdateAPIKey(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()
	defer ts2.Close()

	// get ports from the test servers to make expected data
	addr, _ := url.Parse(ts1.URL)
	_, ts1Port, _ := net.SplitHostPort(addr.Host)
	addr, _ = url.Parse(ts2.URL)
	_, ts2Port, _ := net.SplitHostPort(addr.Host)

	// swap if necessary to ensure ts1 has a smaller port than ts2
	// makes the json marshal deterministic so test is not flakey
	if ts1Port > ts2Port {
		swapPort := ts1Port
		ts1Port = ts2Port
		ts2Port = swapPort
		swapServer := ts1
		ts1 = ts2
		ts2 = swapServer
	}

	// starting API Keys, before the update
	keysPerDomains := map[string][]string{
		ts1.URL: {"api_key1", "api_key2"},
		ts2.URL: {"api_key3"},
	}

	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	cfg := fxutil.Test[config.Component](t, config.MockModule())

	fh := forwarderHealth{log: log, config: cfg, domainResolvers: resolver.NewSingleDomainResolvers(keysPerDomains)}
	fh.init()
	assert.True(t, fh.checkValidAPIKey())

	// forwardHealth's keysPerAPIEndpoint has the given API Keys
	data, _ := json.Marshal(fh.keysPerAPIEndpoint)
	expect := fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key1","api_key2"],"http://127.0.0.1:%s":["api_key3"]}`, ts1Port, ts2Port)
	assert.Equal(t, expect, string(data))

	// update the API Key
	fh.updateAPIKey("api_key1", "api_key4")

	// ensure that keysPerAPIEndpoint has the new API Key
	data, _ = json.Marshal(fh.keysPerAPIEndpoint)
	expect = fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key4","api_key2"],"http://127.0.0.1:%s":["api_key3"]}`, ts1Port, ts2Port)
	assert.Equal(t, expect, string(data))
}
