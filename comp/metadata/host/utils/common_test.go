// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestGetCommonPayload(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.Set("api_key", "foo")

	p := GetCommonPayload("hostname", mockConfig)
	assert.Equal(t, "foo", p.APIKey)
	assert.Equal(t, version.AgentVersion, p.AgentVersion)
	assert.Equal(t, "hostname", p.InternalHostname)
	assert.Equal(t, getUUID(), p.UUID)
}
