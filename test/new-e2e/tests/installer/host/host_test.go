// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSystemdVersion(t *testing.T) {
	t.Run("plain output", func(t *testing.T) {
		version, err := parseSystemdVersion("255\n")
		require.NoError(t, err)
		assert.Equal(t, 255, version)
	})

	t.Run("dynamic loader diagnostic", func(t *testing.T) {
		output := "ERROR: ld.so: object '/run/datadog-apm-inject/launcher.preload.so' from /etc/ld.so.preload cannot be preloaded (cannot open shared object file): ignored.\n255\n"
		version, err := parseSystemdVersion(output)
		require.NoError(t, err)
		assert.Equal(t, 255, version)
	})

	t.Run("missing version", func(t *testing.T) {
		_, err := parseSystemdVersion("systemd version unavailable")
		require.Error(t, err)
	})
}
