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
// variable sources: applicationPoolDefaults and per-pool overrides, plus
// inherited applicationPool resolution.
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

	t.Run("env var names matched case-insensitively", func(t *testing.T) {
		// /g uses poolMixedCase which declares Dd_Service and dd_version
		// (mixed/lowercase). Windows env vars are case-insensitive so these
		// must still populate DD_SERVICE / DD_VERSION.
		_, _, env := iisCfg.GetAPMTags(10, "/g")
		assert.Equal(t, "mixed-case-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "lower-version", env.DDVersion)
	})

	t.Run("app inherits applicationPool from <sites><applicationDefaults>", func(t *testing.T) {
		// /h omits applicationPool; the <sites><applicationDefaults applicationPool="poolB"/>
		// supplies it, so DD_SERVICE/DD_ENV must come from poolB.
		_, _, env := iisCfg.GetAPMTags(10, "/h")
		assert.Equal(t, "poolB-service", env.DDService)
		assert.Equal(t, "poolB-env", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("app inherits applicationPool from per-site <applicationDefaults>", func(t *testing.T) {
		// site 11 has <site><applicationDefaults applicationPool="poolA"/>;
		// its "/" application omits applicationPool. Per-site default wins
		// over the <sites>-level default (which would have chosen poolB).
		_, _, env := iisCfg.GetAPMTags(11, "/")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("<remove> drops an inherited applicationPoolDefaults entry", func(t *testing.T) {
		// poolWithRemove sets DD_SERVICE and removes the inherited DD_ENV
		// from applicationPoolDefaults.
		_, _, env := iisCfg.GetAPMTags(10, "/i")
		assert.Equal(t, "remove-pool-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("<clear/> wipes all inherited applicationPoolDefaults entries", func(t *testing.T) {
		// poolWithClear resets the inherited defaults and then adds DD_SERVICE.
		_, _, env := iisCfg.GetAPMTags(10, "/j")
		assert.Equal(t, "clear-pool-service", env.DDService)
		// DD_ENV was inherited from applicationPoolDefaults; <clear/> drops it.
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("<location><aspNetCore> accepts singular <environmentVariable>", func(t *testing.T) {
		// The aspNetCore schema uses <environmentVariable> as the addElement
		// (vs <add> for applicationPools). /locsingular's location uses the
		// singular form for DD_SERVICE/DD_ENV; we must still pick them up.
		_, _, env := iisCfg.GetAPMTags(10, "/locsingular")
		assert.Equal(t, "singular-service", env.DDService)
		assert.Equal(t, "singular-env", env.DDEnv)
		// DD_VERSION still flows through from poolA.
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("<location><aspNetCore> <remove> is a no-op against pool env", func(t *testing.T) {
		// /locremove's <aspNetCore> block <remove>s DD_ENV (which was never
		// added at the aspNetCore scope) and adds DD_VERSION. Because
		// <remove> here only touches the aspNetCore collection, DD_ENV from
		// applicationPoolDefaults still flows through; DD_VERSION is
		// overridden by the location and DD_SERVICE keeps the poolA value.
		_, _, env := iisCfg.GetAPMTags(10, "/locremove")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "3.0.0", env.DDVersion)
	})

	t.Run("nested <location> blocks cascade by specificity", func(t *testing.T) {
		// envsite/loccascade sets DD_ENV+DD_SERVICE; envsite/loccascade/child
		// overrides DD_SERVICE. The child app must see the child's service
		// but the parent's env -- mirroring how IIS merges nested locations.
		_, _, env := iisCfg.GetAPMTags(10, "/loccascade/child")
		assert.Equal(t, "cascade-child-service", env.DDService)
		assert.Equal(t, "cascade-parent-env", env.DDEnv)
		// DD_VERSION still flows through from poolA.
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("nested <clear/> drops inherited aspNetCore adds, pool env shows through", func(t *testing.T) {
		// envsite/loccascadeclear inherits a parent <location>'s aspNetCore
		// <add name="DD_SERVICE"> and then <clear/>s its own aspNetCore
		// collection. The effective aspNetCore set becomes empty, so the
		// pool-level DD_SERVICE (poolA-service) shows through -- matching
		// what the ASP.NET Core Module would actually apply to the process.
		_, _, env := iisCfg.GetAPMTags(10, "/loccascadeclear/child")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

}

// TestAPMTagsFromGlobalEnvVars covers the "global apphost" env var sources:
// the root <configuration><system.webServer><aspNetCore> element and any
// pathless <location> block. IIS inherits both into every site/application,
// so USM must overlay them onto the pool env before site/app <location>s
// apply — otherwise the .NET tracer sees DD_* in the worker but the USM
// tag tree returns nothing.
func TestAPMTagsFromGlobalEnvVars(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "globalenvvars.xml")
	testroot := filepath.Join(path, "testdata")
	t.Setenv("TESTROOTDIR", testroot)

	iisCfg, err := NewDynamicIISConfig()
	require.Nil(t, err)
	require.NotNil(t, iisCfg)

	require.Nil(t, iisCfg.Start())
	defer iisCfg.Stop()

	t.Run("root <system.webServer> and pathless <location> overlay pool env", func(t *testing.T) {
		// globalsite/ has no app-specific <location>. The root
		// <system.webServer><aspNetCore> contributes DD_SERVICE and the
		// pathless <location> contributes DD_ENV; both override the pool's
		// DD_SERVICE. DD_VERSION still flows from the pool unchanged.
		_, _, env := iisCfg.GetAPMTags(20, "/")
		assert.Equal(t, "root-global-service", env.DDService)
		assert.Equal(t, "pathless-global-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("app-level <location> overrides global per key", func(t *testing.T) {
		// globalsite/override declares its own DD_SERVICE; the global DD_ENV
		// still shows through because the override location only touches
		// DD_SERVICE.
		_, _, env := iisCfg.GetAPMTags(20, "/override")
		assert.Equal(t, "override-service", env.DDService)
		assert.Equal(t, "pathless-global-env", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})
}

// TestApplyEnvVarsOver_OrderSensitivity verifies that <add>/<remove>/<clear>
// inside a single <environmentVariables> block are evaluated in document
// order — so an <add> followed by <clear/> ends up empty, and a <remove>
// after the matching <add> wipes the just-added field.
func TestApplyEnvVarsOver_OrderSensitivity(t *testing.T) {
	t.Run("add then clear yields empty", func(t *testing.T) {
		vars := iisEnvironmentVariables{Ops: []iisEnvVarOp{
			{kind: iisEnvVarOpAdd, name: "DD_SERVICE", value: "x"},
			{kind: iisEnvVarOpClear},
		}}
		out := applyEnvVarsOver(APMTags{DDEnv: "inherited"}, vars)
		assert.Equal(t, APMTags{}, out)
	})

	t.Run("add then remove yields empty for that field", func(t *testing.T) {
		vars := iisEnvironmentVariables{Ops: []iisEnvVarOp{
			{kind: iisEnvVarOpAdd, name: "DD_SERVICE", value: "x"},
			{kind: iisEnvVarOpRemove, name: "DD_SERVICE"},
		}}
		out := applyEnvVarsOver(APMTags{}, vars)
		assert.Equal(t, "", out.DDService)
	})

	t.Run("clear then add keeps the new value", func(t *testing.T) {
		vars := iisEnvironmentVariables{Ops: []iisEnvVarOp{
			{kind: iisEnvVarOpClear},
			{kind: iisEnvVarOpAdd, name: "DD_SERVICE", value: "x"},
		}}
		out := applyEnvVarsOver(APMTags{DDEnv: "inherited"}, vars)
		assert.Equal(t, "x", out.DDService)
		assert.Equal(t, "", out.DDEnv)
	})

	t.Run("remove then re-add restores the field", func(t *testing.T) {
		vars := iisEnvironmentVariables{Ops: []iisEnvVarOp{
			{kind: iisEnvVarOpRemove, name: "DD_SERVICE"},
			{kind: iisEnvVarOpAdd, name: "DD_SERVICE", value: "new"},
		}}
		out := applyEnvVarsOver(APMTags{DDService: "old"}, vars)
		assert.Equal(t, "new", out.DDService)
	})
}

func TestAPMTagsIsEmpty(t *testing.T) {
	assert.True(t, APMTags{}.isEmpty())
	assert.False(t, APMTags{DDService: "x"}.isEmpty())
	assert.False(t, APMTags{DDEnv: "x"}.isEmpty())
	assert.False(t, APMTags{DDVersion: "x"}.isEmpty())
}

// TestPoolDefaultsToDefaultAppPool verifies that an <application> with no
// applicationPool, and no <applicationDefaults> at site or sites level,
// resolves to IIS's hard-coded "DefaultAppPool" — so env vars configured
// on a pool literally named "DefaultAppPool" are picked up.
func TestPoolDefaultsToDefaultAppPool(t *testing.T) {
	cfg := &iisConfiguration{
		ApplicationHost: iisSystemApplicationHost{
			ApplicationPools: iisApplicationPools{
				Pools: []iisApplicationPool{
					{
						Name: "DefaultAppPool",
						EnvVars: iisEnvironmentVariables{
							Ops: []iisEnvVarOp{
								{kind: iisEnvVarOpAdd, name: "DD_SERVICE", value: "default-app-pool-svc"},
								{kind: iisEnvVarOpAdd, name: "DD_VERSION", value: "0.1"},
							},
						},
					},
				},
			},
			Sites: []iisSite{
				{
					SiteID: "100",
					Applications: []iisApplication{
						{
							Path: "/",
							// applicationPool omitted on purpose; no <applicationDefaults> set.
							VirtualDirs: []iisVirtualDirectory{
								{Path: "/", PhysicalPath: "C:\\does-not-exist"},
							},
						},
					},
				},
			},
		},
	}

	trees := buildPathTagTree(cfg)
	_, _, env := findInPathTree(trees, 100, "/")
	assert.Equal(t, "default-app-pool-svc", env.DDService)
	assert.Equal(t, "0.1", env.DDVersion)
}
