// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.
//go:build windows

package iisconfig

import (
	"encoding/xml"
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

	t.Run("pool with own env does not inherit applicationPoolDefaults", func(t *testing.T) {
		// poolA declares its own <environmentVariables>, so it does not inherit
		// applicationPoolDefaults: DD_ENV (defaults-only) is dropped.
		_, _, env := iisCfg.GetAPMTags(10, "/")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("pool sets its own DD_ENV", func(t *testing.T) {
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
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("undeclared pool falls back to applicationPoolDefaults", func(t *testing.T) {
		// /f's "DefaultAppPool" is undeclared (no own <environmentVariables>), so
		// it inherits applicationPoolDefaults.
		_, _, env := iisCfg.GetAPMTags(10, "/f")
		assert.Equal(t, "", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("declared pool that omits env inherits applicationPoolDefaults", func(t *testing.T) {
		// poolNoEnv omits <environmentVariables> entirely (vs an empty one), so
		// it inherits applicationPoolDefaults.
		_, _, env := iisCfg.GetAPMTags(10, "/noenv")
		assert.Equal(t, "", env.DDService)
		assert.Equal(t, "default-env", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("env var names matched case-insensitively", func(t *testing.T) {
		// poolMixedCase declares Dd_Service/dd_version (mixed/lowercase); env var
		// names are case-insensitive so they still populate DD_SERVICE/DD_VERSION.
		_, _, env := iisCfg.GetAPMTags(10, "/g")
		assert.Equal(t, "mixed-case-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
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
		// site 11's per-site <applicationDefaults applicationPool="poolA"/> wins
		// over the <sites>-level default (poolB) for its "/" app.
		_, _, env := iisCfg.GetAPMTags(11, "/")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("<remove> inside a pool collection is a no-op against defaults", func(t *testing.T) {
		// poolWithRemove has its own collection (no defaults inherited); its
		// <remove DD_ENV/> is a no-op, only the <add DD_SERVICE> takes effect.
		_, _, env := iisCfg.GetAPMTags(10, "/i")
		assert.Equal(t, "remove-pool-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "", env.DDVersion)
	})

	t.Run("<clear/> inside a pool collection then add", func(t *testing.T) {
		// poolWithClear declares its own collection (no defaults inherited),
		// <clear/>s it, then adds DD_SERVICE.
		_, _, env := iisCfg.GetAPMTags(10, "/j")
		assert.Equal(t, "clear-pool-service", env.DDService)
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
		// /locremove's aspNetCore adds DD_VERSION (overriding poolA) and <remove>s
		// DD_ENV (a no-op); DD_SERVICE/DD_ENV stay poolA's (poolA has no DD_ENV).
		_, _, env := iisCfg.GetAPMTags(10, "/locremove")
		assert.Equal(t, "poolA-service", env.DDService)
		assert.Equal(t, "", env.DDEnv)
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
		assert.Equal(t, "", env.DDEnv)
		assert.Equal(t, "1.0.0", env.DDVersion)
	})

	t.Run("empty child app does not inherit parent app's pool tags", func(t *testing.T) {
		// emptychild (poolA) -> {poolA-service, "", 1.0.0}; its child runs on
		// poolEmpty (<clear/> -> no DD_*) and must be its own node, not inherit.
		_, _, parent := iisCfg.GetAPMTags(10, "/emptychild")
		assert.Equal(t, "poolA-service", parent.DDService)
		assert.Equal(t, "", parent.DDEnv)
		assert.Equal(t, "1.0.0", parent.DDVersion)

		_, _, child := iisCfg.GetAPMTags(10, "/emptychild/child")
		assert.Equal(t, "", child.DDService)
		assert.Equal(t, "", child.DDEnv)
		assert.Equal(t, "", child.DDVersion)

		// Requests beneath the child hit the child worker; without a child node
		// findInPathTree would fall back to the parent's tags.
		_, _, deep := iisCfg.GetAPMTags(10, "/emptychild/child/sub/path")
		assert.Equal(t, "", deep.DDService)
		assert.Equal(t, "", deep.DDEnv)
		assert.Equal(t, "", deep.DDVersion)
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

// Cross-file precedence: a Core app's web.config <aspNetCore> env overrides the
// app-pool env per field; a Framework app's <appSettings> stays below it.
func TestWebConfigPrecedenceOverApplicationHost(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "webconfig_precedence.xml")
	t.Setenv("TESTROOTDIR", filepath.Join(path, "testdata"))

	iisCfg, err := NewDynamicIISConfig()
	require.Nil(t, err)
	require.NotNil(t, iisCfg)
	require.Nil(t, iisCfg.Start())
	defer iisCfg.Stop()

	t.Run("Core web.config <aspNetCore> overrides applicationHost env", func(t *testing.T) {
		json, config, appHost := iisCfg.GetAPMTags(30, "/core")
		// web.config aspNetCore DD_SERVICE wins; the pool supplies env/version.
		assert.Equal(t, "webconfig-service", appHost.DDService)
		assert.Equal(t, "poolX-env", appHost.DDEnv)
		assert.Equal(t, "poolX-version", appHost.DDVersion)
		// Folded into the env tier, so the separate web.config tier is empty.
		assert.True(t, config.isEmpty())
		assert.True(t, json.isEmpty())
	})

	t.Run("Framework web.config <appSettings> stays below applicationHost env", func(t *testing.T) {
		_, config, appHost := iisCfg.GetAPMTags(30, "/fw")
		// pool env vars are the real environment -> they outrank appSettings.
		assert.Equal(t, "poolX-service", appHost.DDService)
		assert.Equal(t, "poolX-env", appHost.DDEnv)
		assert.Equal(t, "poolX-version", appHost.DDVersion)
		// appSettings stays in its own lower tier (not folded, not dropped).
		assert.Equal(t, "appsettings-service", config.DDService)
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
							// XMLName mirrors the decoder setting it for a declared
							// <environmentVariables> -- the signal buildPoolEnvTags keys off.
							XMLName: xml.Name{Local: "environmentVariables"},
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
