// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
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
	mockConfig := config.NewMock(t)
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
	forwarder := NewDefaultForwarder(mockConfig, log, forwarderOptions)

	// API keys from the domain resolvers match
	expectData := `{"example1.com":["api_key1","api_key2"],"example2.com":["api_key3"]}`
	actualAPIKeys := forwarder.domainAPIKeyMap()
	data, err := json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))

	// update the APIKey by setting it on the config
	mockConfig.Set("api_key", "api_key4", pkgconfigmodel.SourceAgentRuntime)

	assert.Eventually(t, func() bool {
		return assert.Equal(t, mockConfig.Get("api_key"), "api_key4")
	}, 5*time.Second, 200*time.Millisecond)

	// API keys still match after the update
	expectData = `{"example1.com":["api_key4","api_key2"],"example2.com":["api_key3"]}`
	actualAPIKeys = forwarder.domainAPIKeyMap()
	data, err = json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))
}

func TestDefaultForwarderUpdateAdditionalEndpointAPIKey(t *testing.T) {
	mockConfig := config.NewMock(t)
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

	assert.Eventually(t, func() bool {
		return assert.Equal(t, mockConfig.Get("additional_endpoints"), map[string][]string{"example1.com": {"api_key2"}})
	}, 5*time.Second, 200*time.Millisecond)

	// The endpoint has both api keys
	expectData = `{"example1.com":["api_key1","api_key2"],"example2.com":["api_key3"]}`
	actualAPIKeys = forwarder.domainAPIKeyMap()
	data, err = json.Marshal(actualAPIKeys)
	require.NoError(t, err)
	assert.Equal(t, expectData, string(data))
}

func TestPreaggregationPipelineTransactionCreation(t *testing.T) {
	log := logmock.New(t)

	tests := []struct {
		name               string
		preaggrURL         string
		primaryURL         string
		payloadDest        transaction.Destination
		expectTransactions int
		description        string
	}{
		{
			name:               "preaggronly_payload",
			preaggrURL:         "https://telemetry-intake.datadoghq.com",
			primaryURL:         "https://app.datadoghq.com",
			payloadDest:        transaction.PreaggrOnly,
			expectTransactions: 1,
			description:        "PreaggrOnly payload should create preaggr transaction",
		},
		{
			name:               "normal_payload",
			preaggrURL:         "https://telemetry-intake.datadoghq.com",
			primaryURL:         "https://app.datadoghq.com",
			payloadDest:        transaction.AllRegions,
			expectTransactions: 1,
			description:        "Normal payload should create transaction for primary endpoint only, not preaggr",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			mockConfig.Set("api_key", "test_api_key", pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("dd_url", tc.primaryURL, pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("preaggregation.enabled", true, pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("preaggregation.dd_url", tc.preaggrURL, pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("preaggregation.api_key", "preaggr_test_key", pkgconfigmodel.SourceAgentRuntime)

			keysPerDomains, err := utils.GetMultipleEndpoints(mockConfig)
			require.NoError(t, err)

			forwarderOptions, err := NewOptions(mockConfig, log, keysPerDomains)
			require.NoError(t, err)
			forwarder := NewDefaultForwarder(mockConfig, log, forwarderOptions)

			// Create test payload with specified destination
			payload := transaction.NewBytesPayloadWithoutMetaData([]byte(`{"test": "data"}`))
			payload.Destination = tc.payloadDest

			transactions := forwarder.createAdvancedHTTPTransactions(endpoints.SeriesEndpoint, transaction.BytesPayloads{payload}, nil, transaction.TransactionPriorityNormal, transaction.Series, true)

			assert.Equal(t, tc.expectTransactions, len(transactions),
				"Expected %d transactions for %s: %s", tc.expectTransactions, tc.name, tc.description)
		})
	}
}
