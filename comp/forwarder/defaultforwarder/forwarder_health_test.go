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
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestCheckValidAPIKey(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()
	defer ts2.Close()

	keysPerDomains := map[string][]utils.APIKeys{
		ts1.URL: {
			utils.NewAPIKeys("path", "api_key1"),
			utils.NewAPIKeys("path", "api_key2"),
		},
		ts2.URL: {
			utils.NewAPIKeys("path", "key3"),
		},
	}
	log := logmock.New(t)
	cfg := config.NewMock(t)
	r, err := resolver.NewSingleDomainResolvers(keysPerDomains)
	require.NoError(t, err)
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: r}
	fh.init()
	assert.True(t, fh.checkValidAPIKey())

	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key1"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with _key2"))
	assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with key3"))
}

func TestComputeDomainsURL(t *testing.T) {
	keysPerDomains := map[string][]utils.APIKeys{
		"https://app.datadoghq.com":              {utils.NewAPIKeys("path", "api_key1")},
		"https://custom.datadoghq.com":           {utils.NewAPIKeys("path", "api_key2")},
		"https://custom.agent.datadoghq.com":     {utils.NewAPIKeys("path", "api_key3")},
		"https://app.datadoghq.eu":               {utils.NewAPIKeys("path", "api_key4")},
		"https://app.us2.datadoghq.com":          {utils.NewAPIKeys("path", "api_key5")},
		"https://app.xx9.datadoghq.com":          {utils.NewAPIKeys("path", "api_key5")},
		"https://custom.agent.us2.datadoghq.com": {utils.NewAPIKeys("path", "api_key6")},
		// debatable whether the next one should be changed to `api.`, preserve pre-existing behavior for now
		"https://app.datadoghq.internal": {utils.NewAPIKeys("path", "api_key7")},
		"https://app.myproxy.com":        {utils.NewAPIKeys("path", "api_key8")},
		"https://app.ddog-gov.com":       {utils.NewAPIKeys("path", "api_key9")},
		"https://custom.ddog-gov.com":    {utils.NewAPIKeys("path", "api_key10")},
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
	log := logmock.New(t)
	r, err := resolver.NewSingleDomainResolvers(keysPerDomains)
	require.NoError(t, err)
	fh := forwarderHealth{log: log, domainResolvers: r}
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
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ts3 := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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
	log := logmock.New(t)
	cfg := config.NewMock(t)
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
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	keysPerDomains := map[string][]utils.APIKeys{
		ts1.URL: {utils.NewAPIKeys("path1", "api_key1"), utils.NewAPIKeys("path2", "api_key2")},
		ts2.URL: {utils.NewAPIKeys("path", "api_key3")},
	}

	log := logmock.New(t)
	cfg := config.NewMock(t)

	r, err := resolver.NewSingleDomainResolvers(keysPerDomains)
	require.NoError(t, err)
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: r}
	fh.init()
	assert.True(t, fh.checkValidAPIKey())

	// forwardHealth's keysPerAPIEndpoint has the given API Keys
	expect := fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key1","api_key2"],"http://127.0.0.1:%s":["api_key3"]}`, ts1Port, ts2Port)
	assert.Eventually(t, func() bool {
		data, _ := json.Marshal(getKeysCopy(&fh))
		return assert.Equal(t, expect, string(data))
	}, 5*time.Second, 200*time.Millisecond)

	// update the resolver first since the health checker will load the new keys from the resolver
	r[ts1.URL].UpdateAPIKeys("path1", []utils.APIKeys{utils.NewAPIKeys("path1", "api_key4")})
	// update the API Key
	fh.UpdateAPIKeys(ts1.URL, []string{"api_key1"}, []string{"api_key4"})

	expect = fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key2","api_key4"],"http://127.0.0.1:%s":["api_key3"]}`, ts1Port, ts2Port)
	assert.Eventually(t, func() bool {
		// ensure that keysPerAPIEndpoint has the new API Key
		data, _ := json.Marshal(getKeysCopy(&fh))
		return assert.Equal(t, expect, string(data))
	}, 5*time.Second, 200*time.Millisecond)
}

func getKeysCopy(fh *forwarderHealth) map[string][]string {
	fh.keyMapMutex.Lock()
	defer fh.keyMapMutex.Unlock()
	keysMapCopy := make(map[string][]string, len(fh.keysPerAPIEndpoint))
	for k, v := range fh.keysPerAPIEndpoint {
		keysMapCopy[k] = slices.Clone(v)
	}
	return keysMapCopy
}

func quoteList(list []string) string {
	result := []string{}
	for _, item := range list {
		result = append(result, "\""+item+"\"")
	}

	return strings.Join(result, ",")
}

// runUpdateAPIKeysTest test what happens when changing the api keys on additional endpoints.
// The url already has process.additional_endpoints of "api_key1" and "api_key2". When updating these
// keys we need to make sure duplicates with these are well handled.
func runUpdateAPIKeysTest(t *testing.T, description string, keysBefore, keysAfter, expectBefore, expectAfter []string) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	keysPerDomains := map[string][]utils.APIKeys{
		ts1.URL: {utils.NewAPIKeys("api_key", "api_key1")},
		ts2.URL: {
			utils.NewAPIKeys("process.additional_endpoints", "api_key1", "api_key2"),
			utils.NewAPIKeys("additional_endpoints", keysBefore...),
		},
	}

	log := logmock.New(t)
	cfg := config.NewMock(t)

	resolvers, err := resolver.NewSingleDomainResolvers(keysPerDomains)

	for _, r := range resolvers {
		resolver.OnUpdateConfig(r, log, cfg)
	}

	require.NoError(t, err)
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: resolvers}
	fh.init()
	assert.Eventually(t, func() bool {
		return assert.True(t, fh.checkValidAPIKey(), description)
	}, 5*time.Second, 200*time.Millisecond)

	expectBeforeFmt := quoteList(expectBefore)
	expectAfterFmt := quoteList(expectAfter)

	// forwardHealth's keysPerAPIEndpoint has the given API Keys
	expect := fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key1"],"http://127.0.0.1:%s":[%v]}`,
		ts1Port, ts2Port,
		expectBeforeFmt)

	assert.Eventually(t, func() bool {
		data, _ := json.Marshal(getKeysCopy(&fh))
		return assert.Equal(t, expect, string(data), description)
	}, 5*time.Second, 200*time.Millisecond)
	endpoints := map[string][]string{
		ts2.URL: keysAfter,
	}
	// Setting the config will send the change to the resolver, which updates the health check
	cfg.SetWithoutSource("additional_endpoints", endpoints)

	expect = fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key1"],"http://127.0.0.1:%s":[%v]}`,
		ts1Port, ts2Port,
		expectAfterFmt)
	assert.Eventually(t, func() bool {
		data, _ := json.Marshal(getKeysCopy(&fh))
		return assert.Equal(t, expect, string(data), description)
	}, 5*time.Second, 200*time.Millisecond)

	// Check the new keys are now valid
	for _, key := range expectAfter {
		assert.Eventually(t, func() bool {
			return assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with "+key[len(key)-5:]), key)
		}, 5*time.Second, 200*time.Millisecond)
	}

	// Check removed keys are not valid
	for _, key := range expectBefore {
		if !slices.Contains(expectAfter, key) {
			assert.Eventually(t, func() bool {
				return assert.Nil(t, apiKeyStatus.Get("API key ending with "+key[len(key)-5:]), key)
			}, 5*time.Second, 200*time.Millisecond)
		}
	}

	// Restore the keys and ensure they are properly restored
	endpoints = map[string][]string{
		ts2.URL: keysBefore,
	}
	// Setting the config will send the change to the resolver, which updates the health check
	cfg.SetWithoutSource("additional_endpoints", endpoints)

	// Ensure that keysPerAPIEndpoint are restored to previous
	expect = fmt.Sprintf(`{"http://127.0.0.1:%s":["api_key1"],"http://127.0.0.1:%s":[%v]}`,
		ts1Port, ts2Port,
		expectBeforeFmt)
	assert.Eventually(t, func() bool {
		data, _ := json.Marshal(getKeysCopy(&fh))
		return assert.Equal(t, expect, string(data), description)
	}, 5*time.Second, 200*time.Millisecond)

	// Check the old keys are now valid again
	for _, key := range expectBefore {
		assert.Eventually(t, func() bool {
			return assert.Equal(t, &apiKeyValid, apiKeyStatus.Get("API key ending with "+key[len(key)-5:]), key)
		}, 5*time.Second, 200*time.Millisecond)
	}

	// Check added keys are now not valid
	for _, key := range expectAfter {
		if !slices.Contains(expectBefore, key) {
			assert.Eventually(t, func() bool {
				return assert.Nil(t, apiKeyStatus.Get("API key ending with "+key[len(key)-5:]), key)
			}, 5*time.Second, 200*time.Millisecond)
		}
	}
}

func TestConfigUpdateAPIKey(t *testing.T) {
	for _, test := range []struct {
		description  string
		before       []string
		after        []string
		expectBefore []string
		expectAfter  []string
	}{
		{
			description: "Adding api key",
			before:      []string{"api_key1", "api_key3"},
			// The duplicated api_key1 is changed to api_key4
			after:        []string{"api_key3", "api_key4"},
			expectBefore: []string{"api_key1", "api_key2", "api_key3"},
			// We now have both api_key1 and api_key4
			expectAfter: []string{"api_key1", "api_key2", "api_key3", "api_key4"},
		},
		{
			description: "Duplicate keys",
			before:      []string{"api_key3", "api_key4"},
			// api_key4 is changed to the duplicated api_key1
			after:        []string{"api_key3", "api_key1", "api_key1"},
			expectBefore: []string{"api_key1", "api_key2", "api_key3", "api_key4"},
			// api_key1 is still only there once.
			expectAfter: []string{"api_key1", "api_key2", "api_key3"},
		},
		{
			description: "Reducing keys",
			before:      []string{"api_key3", "api_key4"},
			// api_key4 is changed to the duplicated api_key1
			after:        []string{"api_key1", "api_key4"},
			expectBefore: []string{"api_key1", "api_key2", "api_key3", "api_key4"},
			// api_key1 is still only there once.
			expectAfter: []string{"api_key1", "api_key2", "api_key4"},
		},
		{
			description: "All duplicate keys",
			before:      []string{"api_key3", "api_key4"},
			// api_keys are both changed to the process.additional_endpoint keys
			after:        []string{"api_key1", "api_key2"},
			expectBefore: []string{"api_key1", "api_key2", "api_key3", "api_key4"},
			// api_key1 is still only there once.
			expectAfter: []string{"api_key1", "api_key2"},
		},
	} {
		runUpdateAPIKeysTest(t, test.description, test.before, test.after, test.expectBefore, test.expectAfter)
	}
}

func TestOneEndpointNoAPIKeys(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()
	defer ts2.Close()

	// additional endpoints has no API keys, but endpoint should still
	// be valid because the main endpoint does.
	keysPerDomains := map[string][]utils.APIKeys{
		ts1.URL: {utils.NewAPIKeys("api_key", "api_key1")},
		ts2.URL: {
			utils.NewAPIKeys("additional_endpoints"),
		},
	}

	log := logmock.New(t)
	cfg := config.NewMock(t)

	resolvers, err := resolver.NewSingleDomainResolvers(keysPerDomains)

	for _, r := range resolvers {
		resolver.OnUpdateConfig(r, log, cfg)
	}

	require.NoError(t, err)
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: resolvers}
	fh.init()
	assert.True(t, fh.checkValidAPIKey(), "Endpoint should be valid")
}

func TestOneEndpointInvalid(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts1.Close()
	defer ts2.Close()

	// additional endpoints has no API keys, but endpoint should still
	// be valid because the main endpoint does.
	keysPerDomains := map[string][]utils.APIKeys{
		ts1.URL: {utils.NewAPIKeys("api_key", "api_key1")},
		ts2.URL: {
			utils.NewAPIKeys("additional_endpoints", "api_key2"),
		},
	}

	log := logmock.New(t)
	cfg := config.NewMock(t)

	resolvers, err := resolver.NewSingleDomainResolvers(keysPerDomains)

	for _, r := range resolvers {
		resolver.OnUpdateConfig(r, log, cfg)
	}

	require.NoError(t, err)
	fh := forwarderHealth{log: log, config: cfg, domainResolvers: resolvers}
	fh.init()
	assert.True(t, fh.checkValidAPIKey(), "Endpoint should be valid")
}
