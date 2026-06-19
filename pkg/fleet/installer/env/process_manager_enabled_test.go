// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessManagerEnabled(t *testing.T) {
	t.Run("unset defaults true so procmgr hooks run", func(t *testing.T) {
		require.NoError(t, os.Unsetenv(envProcessManagerEnabled))
		e := FromEnv()
		assert.True(t, e.ProcessManagerEnabled)
	})
	t.Run("explicit empty defaults true", func(t *testing.T) {
		t.Setenv(envProcessManagerEnabled, "")
		e := FromEnv()
		assert.True(t, e.ProcessManagerEnabled)
	})
	t.Run("whitespace only defaults true", func(t *testing.T) {
		t.Setenv(envProcessManagerEnabled, "  \t ")
		e := FromEnv()
		assert.True(t, e.ProcessManagerEnabled)
	})
	t.Run("false disables case insensitive", func(t *testing.T) {
		for _, v := range []string{"false", "FALSE", "FaLsE"} {
			t.Run(v, func(t *testing.T) {
				t.Setenv(envProcessManagerEnabled, v)
				assert.False(t, FromEnv().ProcessManagerEnabled)
			})
		}
	})
	t.Run("any other non false value stays enabled", func(t *testing.T) {
		for _, v := range []string{"true", "1", "yes", "0", "no", "off", "anything", "typo"} {
			t.Run(v, func(t *testing.T) {
				t.Setenv(envProcessManagerEnabled, v)
				assert.True(t, FromEnv().ProcessManagerEnabled)
			})
		}
	})
}
