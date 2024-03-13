// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarder

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultForwarderUpdateAPIKey(t *testing.T) {
	// Test default values
	mockConfig := pkgconfigsetup.Conf()
	mockConfig.Set("api_key", "api_key1", pkgconfigmodel.SourceAgentRuntime)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())

	// starting API Keys, before the update
	keysPerDomains := map[string][]string{
		"example1.com": {"api_key1", "api_key2"},
		"example2.com": {"api_key3"},
	}
	forwarderOptions := NewOptions(mockConfig, log, keysPerDomains)
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
