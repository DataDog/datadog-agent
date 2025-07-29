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
		name              string
		enablePreaggr     bool
		preaggrURL        string
		primaryURL        string
		payloadDest       transaction.Destination
		expectTransactions int
		expectEndpoint    transaction.Endpoint
		description       string
	}{
		{
			name:              "preaggr_disabled_preaggronly_payload",
			enablePreaggr:     false,
			preaggrURL:        "",
			primaryURL:        "https://app.datadoghq.com",
			payloadDest:       transaction.PreaggrOnly,
			expectTransactions: 0,
			description:       "PreaggrOnly payload should create no transactions when preaggregation is disabled",
		},
		{
			name:              "same_domain_preaggronly_payload",
			enablePreaggr:     true,
			preaggrURL:        "https://app.datadoghq.com",
			primaryURL:        "https://app.datadoghq.com",
			payloadDest:       transaction.PreaggrOnly,
			expectTransactions: 1,
			expectEndpoint:    endpoints.PreaggrSeriesEndpoint,
			description:       "PreaggrOnly payload should create preaggr transaction when domains match",
		},
		{
			name:              "different_domain_preaggronly_payload",
			enablePreaggr:     true,
			preaggrURL:        "https://preaggr-api.datad0g.com",
			primaryURL:        "https://app.datadoghq.com",
			payloadDest:       transaction.PreaggrOnly,
			expectTransactions: 1,
			expectEndpoint:    endpoints.PreaggrSeriesEndpoint,
			description:       "PreaggrOnly payload should create preaggr transaction with different domains",
		},
		{
			name:              "normal_payload_preaggr_enabled",
			enablePreaggr:     true,
			preaggrURL:        "https://app.datadoghq.com",
			primaryURL:        "https://app.datadoghq.com",
			payloadDest:       transaction.AllRegions,
			expectTransactions: 1,
			expectEndpoint:    endpoints.SeriesEndpoint,
			description:       "Normal payload should create standard transaction regardless of preaggr config",
		},
		{
			name:              "local_payload",
			enablePreaggr:     true,
			preaggrURL:        "https://app.datadoghq.com",
			primaryURL:        "https://app.datadoghq.com",
			payloadDest:       transaction.LocalOnly,
			expectTransactions: 0,
			description:       "LocalOnly payload should not create transactions for remote endpoints",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			mockConfig.Set("api_key", "test_api_key", pkgconfigmodel.SourceAgentRuntime)
			mockConfig.Set("dd_url", tc.primaryURL, pkgconfigmodel.SourceAgentRuntime)
			
			if tc.enablePreaggr {
				mockConfig.Set("preaggregation.enabled", true, pkgconfigmodel.SourceAgentRuntime)
				mockConfig.Set("preaggregation.dd_url", tc.preaggrURL, pkgconfigmodel.SourceAgentRuntime)
				mockConfig.Set("preaggregation.api_key", "preaggr_test_key", pkgconfigmodel.SourceAgentRuntime)
			}

			// Use configuration-based endpoint setup to reproduce real behavior
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

			if tc.expectTransactions > 0 && len(transactions) > 0 {
				assert.Equal(t, tc.expectEndpoint, transactions[0].Endpoint,
					"Expected endpoint %s for %s", tc.expectEndpoint.Name, tc.name)
			}
		})
	}
}

func TestPreaggregationDomainComparison(t *testing.T) {
	log := logmock.New(t)

	// Test specifically for the domain comparison bug mentioned in PLAN.md
	testCases := []struct {
		name               string
		preaggrURL         string
		primaryURL         string
		expectTransactions int
		bugDescription     string
	}{
		{
			name:               "standard_datadog_domain",
			preaggrURL:         "https://app.datadoghq.com",
			primaryURL:         "https://app.datadoghq.com",
			expectTransactions: 1,
			bugDescription:     "Primary domain gets agent version prefix (7-59-0-app.agent.datadoghq.com) but preaggregation.dd_url remains raw",
		},
		{
			name:               "custom_domain_no_prefix",
			preaggrURL:         "https://custom.example.com",
			primaryURL:         "https://custom.example.com",
			expectTransactions: 1,
			bugDescription:     "Custom domains without datadog pattern should not get agent version prefix",
		},
		{
			name:               "eu_datadog_domain",
			preaggrURL:         "https://app.datadoghq.eu",
			primaryURL:         "https://app.datadoghq.eu",
			expectTransactions: 1,
			bugDescription:     "EU domains may also experience the agent version prefix issue",
		},
	}

	for _, tc := range testCases {
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

			payload := transaction.NewBytesPayloadWithoutMetaData([]byte(`{"series": []}`))
			payload.Destination = transaction.PreaggrOnly

			transactions := forwarder.createAdvancedHTTPTransactions(endpoints.SeriesEndpoint, transaction.BytesPayloads{payload}, nil, transaction.TransactionPriorityNormal, transaction.Series, true)

			if len(transactions) != tc.expectTransactions {
				t.Logf("UNEXPECTED BEHAVIOR for %s: %s", tc.name, tc.bugDescription)
				t.Logf("Expected %d transactions, got %d", tc.expectTransactions, len(transactions))
				
				// For debugging: print actual domain resolution
				for domain, dr := range forwarder.domainResolvers {
					resolved, _ := dr.Resolve(endpoints.SeriesEndpoint)
					t.Logf("Domain resolver: %s -> %s", domain, resolved)
				}
				t.Logf("preaggregation.dd_url config: %s", mockConfig.GetString("preaggregation.dd_url"))
			}
			
			assert.Equal(t, tc.expectTransactions, len(transactions),
				"Transaction count mismatch for %s: %s", tc.name, tc.bugDescription)
		})
	}
}

func TestPreaggregationConfigurationEdgeCases(t *testing.T) {
	log := logmock.New(t)

	tests := []struct {
		name           string
		setupConfig    func(config.Component)
		payloadDest    transaction.Destination
		expectError    bool
		expectTxns     int
		description    string
	}{
		{
			name: "missing_preaggregation_api_key",
			setupConfig: func(cfg config.Component) {
				cfg.Set("api_key", "main_key", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.enabled", true, pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.dd_url", "https://preaggr.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
				// Deliberately omit preaggregation.api_key
			},
			payloadDest: transaction.PreaggrOnly,
			expectError: false,
			expectTxns:  1,
			description: "Missing preaggr API key behavior - may create transactions for different domain",
		},
		{
			name: "empty_preaggr_url",
			setupConfig: func(cfg config.Component) {
				cfg.Set("api_key", "main_key", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.enabled", true, pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.dd_url", "", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.api_key", "preaggr_key", pkgconfigmodel.SourceAgentRuntime)
			},
			payloadDest: transaction.PreaggrOnly,
			expectError: false,
			expectTxns:  0,
			description: "Empty preaggr URL should not create transactions",
		},
		{
			name: "malformed_preaggr_url",
			setupConfig: func(cfg config.Component) {
				cfg.Set("api_key", "main_key", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.enabled", true, pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.dd_url", "not-a-valid-url", pkgconfigmodel.SourceAgentRuntime)
				cfg.Set("preaggregation.api_key", "preaggr_key", pkgconfigmodel.SourceAgentRuntime)
			},
			payloadDest: transaction.PreaggrOnly,
			expectError: false,
			expectTxns:  1,  
			description: "Malformed preaggr URL behavior - creates transactions for different domain",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			tc.setupConfig(mockConfig)

			keysPerDomains, err := utils.GetMultipleEndpoints(mockConfig)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			forwarderOptions, err := NewOptions(mockConfig, log, keysPerDomains)
			require.NoError(t, err)
			forwarder := NewDefaultForwarder(mockConfig, log, forwarderOptions)

			payload := transaction.NewBytesPayloadWithoutMetaData([]byte(`{"test": "data"}`))
			payload.Destination = tc.payloadDest

			transactions := forwarder.createAdvancedHTTPTransactions(endpoints.SeriesEndpoint, transaction.BytesPayloads{payload}, nil, transaction.TransactionPriorityNormal, transaction.Series, true)

			assert.Equal(t, tc.expectTxns, len(transactions), tc.description)
		})
	}
}
