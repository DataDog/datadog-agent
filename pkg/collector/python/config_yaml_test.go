// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// yaml v3 still resolves YAML 1.1 booleans in typed bool fields, so these common options keep accepting them.
func TestCommonInstanceConfigYAML11BooleanCompat(t *testing.T) {
	var config integration.CommonInstanceConfig
	require.NoError(t, yaml.Unmarshal([]byte("empty_default_hostname: yes\nno_index: 'off'\n"), &config))
	assert.True(t, config.EmptyDefaultHostname)
	assert.False(t, config.NoIndex)

	config = integration.CommonInstanceConfig{}
	require.NoError(t, yaml.Unmarshal([]byte("empty_default_hostname: TRUE\nno_index: false\n"), &config))
	assert.True(t, config.EmptyDefaultHostname)
	assert.False(t, config.NoIndex)
}
