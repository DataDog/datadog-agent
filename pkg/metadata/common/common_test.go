// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	apiKey = "foo"
	p := GetPayload("hostname")
	assert.Equal(t, p.APIKey, "foo")
	assert.Equal(t, p.AgentVersion, version.AgentVersion)
	assert.Equal(t, p.InternalHostname, "hostname")
	apiKey = ""
}

func TestGetAPIKey(t *testing.T) {
	mockConfig := config.NewMock()
	mockConfig.Set("api_key", "bar,baz")
	assert.Equal(t, "bar", getAPIKey())
	assert.Equal(t, "bar", apiKey)
	apiKey = "foo"
	assert.Equal(t, "foo", getAPIKey())
}
