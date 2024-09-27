// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/fatih/color"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type configType string

const (
	metricsConfig configType = "metrics"
	logsConfig    configType = "logs"
)

func TestPrintConfigCheck(t *testing.T) {
	integrationConfigCheckResponse := integration.ConfigCheckResponse{
		Configs: []integration.Config{
			{
				Instances:    []integration.Data{integration.Data("{foo:bar}")},
				InitConfig:   integration.Data("{baz:qux}"),
				LogsConfig:   integration.Data("[{\"service\":\"some_service\",\"source\":\"some_source\"}]"),
				MetricConfig: integration.Data("[{\"metric\":\"some_metric\"}]"),
			},
		},
		ConfigErrors: map[string]string{
			"some_identifier": "some_error",
		},
		ResolveWarnings: map[string][]string{
			"some_identifier": {"some_warning"},
		},
		Unresolved: map[string][]integration.Config{
			"unresolved_config": {
				{
					Instances: []integration.Data{integration.Data("{unresolved:sad}")},
				},
			},
		},
	}

	testCases := []struct {
		name      string
		withDebug bool
		withColor bool
		expected  string
	}{
		{
			name:      "With debug",
			withDebug: true,
			expected: `=== Configuration errors ===

some_identifier: some_error

===  check ===
Configuration provider: Unknown provider
Configuration source: Unknown configuration source
Config for instance ID: :3c3de4a3617771b4
{foo:bar}
~
Init Config:
{baz:qux}
Metric Config:
[{"metric":"some_metric"}]
Log Config:
[{"service":"some_service","source":"some_source"}]
===

=== Resolve warnings ===

some_identifier
* some_warning

=== Unresolved Configs ===

Auto-discovery IDs: unresolved_config
Templates:
check_name: ""
init_config: null
instances:
- unresolved:sad: null
logs_config: null

`,
		},
		{
			name: "Without debug",
			expected: `=== Configuration errors ===

some_identifier: some_error

===  check ===
Configuration provider: Unknown provider
Configuration source: Unknown configuration source
Config for instance ID: :3c3de4a3617771b4
{foo:bar}
~
Init Config:
{baz:qux}
Metric Config:
[{"metric":"some_metric"}]
Log Config:
[{"service":"some_service","source":"some_source"}]
===
`,
		},
		{
			name:      "With color",
			withColor: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var writer io.Writer
			var b bytes.Buffer

			if test.withColor {
				color.Output = &b
				writer = color.Output
				originalNoColor := color.NoColor
				color.NoColor = false
				defer func() {
					color.NoColor = originalNoColor
				}()
			} else {
				writer = &b
			}

			PrintConfigCheck(writer, integrationConfigCheckResponse, test.withDebug)

			if test.withColor {
				// We assert that an ANSI color code is present in the output
				// Using raw string literal ` escapes the escape character `\`
				// which makes the assertion a bit more complex
				assert.Contains(t, b.String(), "\x1b[31m")
			} else {
				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(test.expected, "\r\n", "\n", -1)
				output := strings.Replace(b.String(), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			}
		})
	}
}

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
