// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package iisconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppConfig(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	apppath := filepath.Join(path, "testdata", "app_1.config.xml")
	//apppath := filepath.Join(path, "testdata", "iisconfig.xml")
	envTags, appSettingsTags, err := ReadDotNetConfig(apppath)
	assert.Nil(t, err)
	// appSettings-only (no <aspNetCore>) -> .NET Framework tier, no env tier.
	assert.True(t, envTags.isEmpty())

	assert.Equal(t, appSettingsTags.DDService, "service1")
	assert.Equal(t, appSettingsTags.DDEnv, "false")
	assert.Equal(t, appSettingsTags.DDVersion, "1.0-prerelease")

}

// A Core web.config (<aspNetCore>) derives UST from its env vars only, ignoring
// <appSettings> -- matching the tracer, which never reads appSettings on Core.
func TestAppConfigEnvironmentVariables(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	apppath := filepath.Join(path, "testdata", "app_envvars.config.xml")
	envTags, appSettingsTags, err := ReadDotNetConfig(apppath)
	assert.Nil(t, err)
	// Core (<aspNetCore>) -> env tier only, no appSettings tier.
	assert.True(t, appSettingsTags.isEmpty())

	assert.Equal(t, "from-env", envTags.DDService) // env wins over appSettings
	assert.Equal(t, "", envTags.DDEnv)             // appSettings-only -> dropped on Core
	assert.Equal(t, "2.0-env", envTags.DDVersion)  // env only
}

// On the Framework path, <appSettings> outranks the DD_TRACE_CONFIG_FILE
// datadog.json, which still fills fields appSettings left unset.
func TestAppConfigAppSettingsOutranksDatadogJSON(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	apppath := filepath.Join(path, "testdata", "app_framework.config.xml")
	envTags, appSettingsTags, err := ReadDotNetConfig(apppath)
	assert.Nil(t, err)
	// Framework (<appSettings>) -> appSettings tier only, no env tier.
	assert.True(t, envTags.isEmpty())

	assert.Equal(t, "appsettings-service", appSettingsTags.DDService) // both -> appSettings wins
	assert.Equal(t, "appsettings-version", appSettingsTags.DDVersion) // both -> appSettings wins
	assert.Equal(t, "json-env", appSettingsTags.DDEnv)                // json fills the gap
}

func TestPathSplitting(t *testing.T) {

	t.Run("Test Root path", func(t *testing.T) {
		sp := splitPaths("/")
		assert.Equal(t, 0, len(sp))
	})

	t.Run("Test path depth 3", func(t *testing.T) {
		sp := splitPaths("/path/to/app")
		assert.Equal(t, 3, len(sp))
		assert.Equal(t, "path", sp[0])
		assert.Equal(t, "to", sp[1])
		assert.Equal(t, "app", sp[2])
	})

	t.Run("Test path depth 3 with trailing slash", func(t *testing.T) {
		sp := splitPaths("/path/to/app/")
		assert.Equal(t, 3, len(sp))
		assert.Equal(t, "path", sp[0])
		assert.Equal(t, "to", sp[1])
		assert.Equal(t, "app", sp[2])
	})

}

func TestAPMTags(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "apptest.xml")
	testroot := filepath.Join(path, "testdata")
	os.Setenv("TESTROOTDIR", testroot)
	defer os.Unsetenv("TESTROOTDIR")

	iisCfg, err := NewDynamicIISConfig()
	assert.Nil(t, err)
	assert.NotNil(t, iisCfg)

	err = iisCfg.Start()
	assert.Nil(t, err)

	t.Run("Test simple root path", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("Test deeper path from top app", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/path/to/app")
		assert.Equal(t, "app1", tags.DDService)
	})

	t.Run("test top level app2", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/app2")
		assert.Equal(t, "app2", tags.DDService)
	})
	t.Run("test deeper app2", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/app2/some/path")
		assert.Equal(t, "app2", tags.DDService)
	})

	t.Run("test app3 nested in app2", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/app2/app3")
		assert.Equal(t, "app3", tags.DDService)
	})
	t.Run("test app3 nested in app2 with path", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(2, "/app2/app3/some/path")
		assert.Equal(t, "app3", tags.DDService)
	})

	t.Run("test secondary site", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(3, "/")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("test secondary site app 3", func(t *testing.T) {
		// this should still be app1 because the root path on the
		// second site is different
		tags, _, _ := iisCfg.GetAPMTags(3, "/app2/app3")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("test secondary site actual app3", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(3, "/siteapp2/siteapp3")
		assert.Equal(t, "app3", tags.DDService)
	})
	t.Run("test secondary site actual app3 with file", func(t *testing.T) {
		tags, _, _ := iisCfg.GetAPMTags(3, "/siteapp2/siteapp3/somefile")
		assert.Equal(t, "app3", tags.DDService)
	})
}
