// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestRedactionConfig(t *testing.T) {
	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid: 42,
			Env: map[string]string{
				envRedactedIdentifiers:          "myToken, internal_key",
				envRedactedTypes:                "main.Secret*",
				envRedactionExcludedIdentifiers: "token",
			},
		},
		{Pid: 43},
	})

	t.Run("env extends and excludes defaults", func(t *testing.T) {
		c := redactionConfig(42, procRoot)
		require.True(t, c.RedactIdentifier("my_token"), "user identifier")
		require.True(t, c.RedactIdentifier("internalKey"), "user identifier")
		require.True(t, c.RedactType("main.SecretKey"), "user type prefix")
		require.True(t, c.RedactIdentifier("password"), "default still applies")
		require.False(t, c.RedactIdentifier("token"), "excluded default")
	})

	t.Run("defaults apply without env", func(t *testing.T) {
		c := redactionConfig(43, procRoot)
		require.True(t, c.RedactIdentifier("password"))
		require.False(t, c.RedactIdentifier("mytoken"))
	})

	t.Run("missing process falls back to defaults", func(t *testing.T) {
		c := redactionConfig(99, procRoot)
		require.True(t, c.RedactIdentifier("password"))
	})
}
