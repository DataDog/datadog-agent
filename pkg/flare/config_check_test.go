// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type configType string

const (
	metricsConfig configType = "metrics"
	logsConfig    configType = "logs"
)

func TestContainerExclusionRulesInfo(t *testing.T) {
	outputMsgs := map[configType]string{
		metricsConfig: "This configuration matched a metrics container-exclusion rule, so it will not be run by the Agent",
		logsConfig:    "This configuration matched a logs container-exclusion rule, so it will not be run by the Agent",
	}

	testCases := []struct {
		name       string
		configType configType
		excluded   bool
		expectMsg  bool
	}{
		{
			name:       "Is check config and matches exclusion rule",
			configType: metricsConfig,
			excluded:   true,
			expectMsg:  true,
		},
		{
			name:       "Is check config and does not match exclusion rule",
			configType: metricsConfig,
			excluded:   false,
			expectMsg:  false,
		},
		{
			name:       "Is log config and matches exclusion rule",
			configType: logsConfig,
			excluded:   true,
			expectMsg:  true,
		},
		{
			name:       "Is log config and does not match exclusion rule",
			configType: logsConfig,
			excluded:   false,
			expectMsg:  false,
		},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, test.name), func(t *testing.T) {
			config := newConfig(test.configType, test.excluded)

			var result bytes.Buffer
			PrintConfig(&result, config, "")

			if test.expectMsg {
				assert.Contains(t, result.String(), outputMsgs[test.configType])
			} else {
				assert.NotContains(t, result.String(), outputMsgs[test.configType])
			}
		})
	}
}

func newConfig(configType configType, excluded bool) integration.Config {
	var config integration.Config

	switch configType {
	case metricsConfig:
		config = integration.Config{
			Instances:       []integration.Data{integration.Data("{foo:bar}")},
			ADIdentifiers:   []string{"some_identifier"},
			ClusterCheck:    false,
			MetricsExcluded: excluded,
		}
	case logsConfig:
		config = integration.Config{
			LogsConfig:    integration.Data("[{\"service\":\"some_service\",\"source\":\"some_source\"}]"),
			ADIdentifiers: []string{"some_identifier"},
			LogsExcluded:  excluded,
		}
	}

	return config
}
