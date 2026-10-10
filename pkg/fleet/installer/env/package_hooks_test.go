// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageHookTimeout(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", 5 * time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"1s", time.Second},
		{"10m", 10 * time.Minute},
		{"1h", time.Hour},
	} {
		t.Run(tc.value, func(t *testing.T) {
			config := &Env{PackageHookTimeout: tc.value}
			actual, err := config.GetPackageHookTimeout()
			require.NoError(t, err)
			assert.Equal(t, tc.want, actual)
		})
	}
	for _, invalid := range []string{"invalid", "0", "0s", "-1s", "1h1s", "99999999999999h"} {
		t.Run(invalid, func(t *testing.T) {
			_, err := (&Env{PackageHookTimeout: invalid}).GetPackageHookTimeout()
			require.Error(t, err)
		})
	}
}

func TestPackageHookTimeoutEnvironment(t *testing.T) {
	t.Setenv("DD_INSTALLER_PACKAGE_HOOK_TIMEOUT", "30s")
	config := FromEnv()
	assert.Equal(t, "30s", config.PackageHookTimeout)
	assert.Contains(t, config.ToEnv(), "DD_INSTALLER_PACKAGE_HOOK_TIMEOUT=30s")
	assert.NotContains(t, (&Env{}).ToEnv(), "DD_INSTALLER_PACKAGE_HOOK_TIMEOUT=")
}
