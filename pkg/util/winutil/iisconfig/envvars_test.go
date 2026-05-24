// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.
//go:build windows

package iisconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPMTagsFromEnvVars covers the applicationHost.config environment
// variable sources: applicationPoolDefaults, per-pool, and per-application
// overrides.
func TestAPMTagsFromEnvVars(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "envvars.xml")
	testroot := filepath.Join(path, "testdata")
	t.Setenv("TESTROOTDIR", testroot)

	iisCfg, err := NewDynamicIISConfig()
	require.Nil(t, err)
	require.NotNil(t, iisCfg)

	require.Nil(t, iisCfg.Start())
	defer iisCfg.Stop()

	t.Run("pool env overlays applicationPoolDefaults", func(t *testing.T) {
		// site 10, app "/" uses poolA: SERVICE/VERSION from pool, ENV from
		// applicationPoolDefaults.
		_, _, env := iisCfg.GetAPMTags(10, "/")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("pool overrides applicationPoolDefaults", func(t *testing.T) {
		// poolB sets DD_ENV explicitly, overriding the default.
		_, _, env := iisCfg.GetAPMTags(10, "/b")
		assert.Equal(t, "poolB-service", env.DDService)
		assert.Equal(t, "poolB-env", env.DDEnv)
		// DD_VERSION not set anywhere applicable -> empty.
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("app env overrides pool env", func(t *testing.T) {
		// poolC defines no env vars; app /c sets SERVICE and VERSION at the
		// application level.
		_, _, env := iisCfg.GetAPMTags(10, "/c")
		assert.Equal(t, "app-c-service", env.DDService)
		// DD_ENV comes from applicationPoolDefaults.
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "9.9.9", env.DDVersion)
	})

	t.Run("app inherits only applicationPoolDefaults when pool has nothing", func(t *testing.T) {
		// poolC has no env vars and /d does not override; only defaults apply.
		_, _, env := iisCfg.GetAPMTags(10, "/d")
		assert.Equal(t, "", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("pool lookup is case-insensitive", func(t *testing.T) {
		// /e references "POOLA" but the pool is declared as "poolA".
		// IIS treats pool names case-insensitively.
		_, _, env := iisCfg.GetAPMTags(10, "/e")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("undeclared pool falls back to applicationPoolDefaults", func(t *testing.T) {
		// /f references "DefaultAppPool" which is not listed under
		// <applicationPools><add>. IIS still applies applicationPoolDefaults.
		_, _, env := iisCfg.GetAPMTags(10, "/f")
		assert.Equal(t, "", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})
}

func TestOverlayAPMTags(t *testing.T) {
	base := APMTags{DDService: "base-svc", DDEnv: "base-env", DDVersion: "base-ver"}
	override := APMTags{DDService: "ovr-svc", DDVersion: ""}

	out := overlayAPMTags(base, override)
	assert.Equal(t, "ovr-svc", out.DDService)
	assert.Equal(t, "base-env", out.DDEnv)
	// empty override field must not clear the base value
	assert.Equal(t, "base-ver", out.DDVersion)
}

func TestAPMTagsIsEmpty(t *testing.T) {
	assert.True(t, APMTags{}.isEmpty())
	assert.False(t, APMTags{DDService: "x"}.isEmpty())
	assert.False(t, APMTags{DDEnv: "x"}.isEmpty())
	assert.False(t, APMTags{DDVersion: "x"}.isEmpty())
}
