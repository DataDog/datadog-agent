// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package inventorychecksimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var jmxFetchConfigInstances = `instances:
  - host: localhost
    port: 9999
    tags:
      - env:test`

func Test_inventorychecksImpl_getJMXChecksMetadata(t *testing.T) {
	ic := getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)

	// GIVEN a JMXFetch config scheduled
	jmxfetch.AddScheduledConfig(integration.Config{
		Name:      "JMXFetch_check",
		Instances: []integration.Data{integration.Data(jmxFetchConfigInstances)},
	})

	// AND JMX Status set
	jmx.SetStatus(jmx.Status{
		Info: map[string]interface{}{
			"version":         "1.0.1",
			"runtime_version": "1.2.3",
		},
	})
	t.Cleanup(jmx.ClearStatus)

	// When I get the JMX checks metatdata
	jmxMetadata := ic.getJMXChecksMetadata(true)

	// Then the metadata should be populated
	require.NotEmpty(t, jmxMetadata)
	jCheck := jmxMetadata["JMXFetch_check"]
	require.NotEmpty(t, jCheck)
	assert.Equal(t,
		"1.0.1", jCheck[0]["jmxfetch.version"],
		"Could not get JMXFetch version", jCheck[0])
	assert.Equal(t,
		"1.2.3", jCheck[0]["java.version"],
		"Could not get Java version", jCheck[0])
	assert.Contains(t, jCheck[0], "init_config")
	assert.Contains(t, jCheck[0], "instance")

	// And without the configs, only the identifying fields are returned
	jCheckNoConfigs := ic.getJMXChecksMetadata(false)["JMXFetch_check"]
	require.NotEmpty(t, jCheckNoConfigs)
	assert.NotContains(t, jCheckNoConfigs[0], "init_config")
	assert.NotContains(t, jCheckNoConfigs[0], "instance")
	assert.Equal(t, "1.0.1", jCheckNoConfigs[0]["jmxfetch.version"])
}
