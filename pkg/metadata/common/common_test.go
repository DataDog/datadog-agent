// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestGetPayload(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.Set("api_key", "foo")

	p := GetPayload("hostname")
	assert.Equal(t, p.APIKey, "foo")
	assert.Equal(t, p.AgentVersion, version.AgentVersion)
	assert.Equal(t, p.InternalHostname, "hostname")
}
