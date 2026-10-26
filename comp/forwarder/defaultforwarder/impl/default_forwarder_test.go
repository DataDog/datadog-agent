// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// domainAPIKeyMap used by tests to get API keys from each domain resolver
func (f *DefaultForwarder) domainAPIKeyMap() map[string][]string {
	apiKeyMap := map[string][]string{}
	for domain, dr := range f.domainResolvers {
		apiKeyMap[domain] = dr.GetAPIKeys()
	}
	return apiKeyMap
}

func TestDefaultForwarderUpdateAPIKey(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("api_key", "api_key1", pkgconfigmodel.SourceAgentRuntime)
	log := logmock.New(t)

	// starting API Keys, before the update
	keysPerDomains := map[string][]utils.APIKeys{
		"example1.com": {
			utils.NewAPIKeys("api_key", "api_key1"),
			utils.NewAPIKeys("additional_endpoints", "api_key2"),
		},
		"example2.com": {
			utils.NewAPIKeys("additional_endpoints", "api_key3"),
		},
	}
	forwarderOptions, err := NewOptions(mockConfig, log, keysPerDomains)
	require.NoError(t, err)
	secrets := secretsmock.New(t)
	forwarderOptions.Secrets = secrets
	forwarder := NewDefaultForwarder(mockConfig, log, forwarderOptions)

	// API keys from the domain resolvers match
	expectData := `{"example1.com":["api_key1","api_key2"],"example2.com":["api_key3"]}`
	actualAPIKeys := forwarder.domainAPIKeyMap()
	data, err := json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))

	// update the APIKey by setting it on the config
	mockConfig.Set("api_key", "api_key4", pkgconfigmodel.SourceAgentRuntime)

	// API keys still match after the update
	expectData = `{"example1.com":["api_key4","api_key2"],"example2.com":["api_key3"]}`
	actualAPIKeys = forwarder.domainAPIKeyMap()
	data, err = json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))
}

func TestDefaultForwarderUpdateAdditionalEndpointAPIKey(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("api_key", "api_key1", pkgconfigmodel.SourceAgentRuntime)
	log := logmock.New(t)

	// starting API Keys, before the update
	// main api_key is a duplicate of the additional_endpoints one
	keysPerDomains := map[string][]utils.APIKeys{
		"example1.com": {
			utils.NewAPIKeys("api_key", "api_key1"),
			utils.NewAPIKeys("additional_endpoints", "api_key1"),
		},
		"example2.com": {
			utils.NewAPIKeys("additional_endpoints", "api_key3"),
		},
	}
	forwarderOptions, err := NewOptions(mockConfig, log, keysPerDomains)
	require.NoError(t, err)
	secrets := secretsmock.New(t)
	forwarderOptions.Secrets = secrets
	forwarder := NewDefaultForwarder(mockConfig, log, forwarderOptions)

	// API keys from the domain resolvers match
	expectData := `{"example1.com":["api_key1"],"example2.com":["api_key3"]}`
	actualAPIKeys := forwarder.domainAPIKeyMap()
	data, err := json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))

	// update the APIKey by setting it on the config, we now have a new API key
	mockConfig.Set("additional_endpoints",
		map[string][]string{"example1.com": {"api_key2"}},
		pkgconfigmodel.SourceAgentRuntime,
	)

	// The updated endpoint has both API keys, while the removed endpoint loses its stale key.
	expectData = `{"example1.com":["api_key1","api_key2"],"example2.com":[]}`
	actualAPIKeys = forwarder.domainAPIKeyMap()
	data, err = json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))
}

func TestProcessTransactionReportsSentCredentialAfterRotation(t *testing.T) {
	requestKeys := make(chan string, 2)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		requestKeys <- r.Header.Get("DD-Api-Key")
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer server.Close()

	cfg := configmock.New(t)
	cfg.Set("api_key", "old-key", pkgconfigmodel.SourceAgentRuntime)
	log := logmock.New(t)
	options, err := NewOptions(cfg, log, map[string][]utils.APIKeys{
		server.URL: {utils.NewAPIKeys("api_key", "old-key")},
	})
	require.NoError(t, err)
	options.Secrets = secretsmock.New(t)
	forwarder := NewDefaultForwarder(cfg, log, options)

	txn := transaction.NewHTTPTransaction()
	txn.Kind = transaction.Process
	txn.Domain = server.URL
	txn.Endpoint.Route = "/api/v1/collector"
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("payload"))
	txn.Resolver = forwarder.domainResolvers[server.URL]

	// The transaction was queued under the old key, but resolves its key when sent.
	cfg.Set("api_key", "first-rotated-key", pkgconfigmodel.SourceAgentRuntime)
	client := &http.Client{}
	require.Error(t, txn.Process(context.Background(), cfg, log, options.Secrets, client, nil))
	assert.Equal(t, "first-rotated-key", <-requestKeys)
	assert.Equal(t, 1, txn.APIKeyVersion)

	// A retry of the same transaction must report the key used by its next attempt.
	cfg.Set("api_key", "second-rotated-key", pkgconfigmodel.SourceAgentRuntime)
	require.NoError(t, txn.Process(context.Background(), cfg, log, options.Secrets, client, nil))
	assert.Equal(t, "second-rotated-key", <-requestKeys)
	assert.Equal(t, 2, txn.APIKeyVersion)
}
