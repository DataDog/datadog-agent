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

// TestAPMTagsFromEnvVars covers the new applicationHost.config environment
// variable sources: pool defaults, per-pool, per-application overrides, and
// the system env fallback.
func TestAPMTagsFromEnvVars(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "envvars.xml")
	testroot := filepath.Join(path, "testdata")
	t.Setenv("TESTROOTDIR", testroot)

	// system-level env vars: should populate any field a more specific source
	// does not provide.
	t.Setenv("DD_SERVICE", "system-service")
	t.Setenv("DD_ENV", "system-env")
	t.Setenv("DD_VERSION", "system-version")

	iisCfg, err := NewDynamicIISConfig()
	require.Nil(t, err)
	require.NotNil(t, iisCfg)

	require.Nil(t, iisCfg.Start())
	defer iisCfg.Stop()

	t.Run("pool env overrides pool defaults; system env fills missing fields", func(t *testing.T) {
		// site 10, app "/" uses poolA: SERVICE/VERSION from pool, ENV from
		// applicationPoolDefaults (which beats system DD_ENV).
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
		// version was not set in poolB nor in defaults, falls back to system env.
		assert.Equal(t, "system-version", env.DDVersion)
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

	t.Run("system env fallback when pool has no overrides", func(t *testing.T) {
		// poolC has no env vars and /d does not override; only the defaults
		// (DD_ENV) and system env apply.
		_, _, env := iisCfg.GetAPMTags(10, "/d")
		assert.Equal(t, "system-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "system-version", env.DDVersion)
	})
}

func TestAPMTagsFromEnvVars_NoSystemEnv(t *testing.T) {
	// Ensure no leakage from the host runner.
	t.Setenv("DD_SERVICE", "")
	t.Setenv("DD_ENV", "")
	t.Setenv("DD_VERSION", "")

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

	// poolC has no overrides and the system has nothing set: env tags should
	// only carry the applicationPoolDefaults DD_ENV.
	_, _, env := iisCfg.GetAPMTags(10, "/d")
	assert.Equal(t, "", env.DDService)
	assert.Equal(t, "default-env", env.DDEnv)
	assert.Equal(t, "", env.DDVersion)
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
