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
	"go.yaml.in/yaml/v4"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// TestCommonInstanceConfigYAML11BooleanCompat pins the decode behavior that
// check.go relies on since its switch from go.yaml.in/yaml/v2 to
// go.yaml.in/yaml/v4. yaml v3 and v4 keep the YAML 1.1 compatibility of the
// old parser when unmarshaling into typed bool fields, so the common options
// empty_default_hostname and no_index keep accepting the legacy spellings.
// Generic map reads are the ones that lost them (see TestIsJMXInstance and
// TestCollectDefaultMetricsLegacyBooleans in pkg/collector/check).
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
