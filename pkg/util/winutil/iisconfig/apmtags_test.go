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
	iisCfg, err := ReadDotNetConfig(apppath)
	assert.Nil(t, err)
	assert.NotNil(t, iisCfg)

	assert.Equal(t, iisCfg.DDService, "service1")
	assert.Equal(t, iisCfg.DDEnv, "false")
	assert.Equal(t, iisCfg.DDVersion, "1.0-prerelease")

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

	//apppath := filepath.Join(path, "testdata", "iisconfig.xml")
	iisCfg, err := NewDynamicIISConfig()
	assert.Nil(t, err)
	assert.NotNil(t, iisCfg)

	err = iisCfg.Start()
	assert.Nil(t, err)

	iisCfg.buildPathTagTree()

	t.Run("Test simple root path", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("Test deeper path from top app", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/path/to/app")
		assert.Equal(t, "app1", tags.DDService)
	})

	t.Run("test top level app2", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/app2")
		assert.Equal(t, "app2", tags.DDService)
	})
	t.Run("test deeper app2", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/app2/some/path")
		assert.Equal(t, "app2", tags.DDService)
	})

	t.Run("test app3 nested in app2", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/app2/app3")
		assert.Equal(t, "app3", tags.DDService)
	})
	t.Run("test app3 nested in app2 with path", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("2", "/app2/app3/some/path")
		assert.Equal(t, "app3", tags.DDService)
	})

	t.Run("test secondary site", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("3", "/")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("test secondary site app 3", func(t *testing.T) {
		// this should still be app1 because the root path on the
		// second site is different
		tags, _ := iisCfg.GetAPMTags("3", "/app2/app3")
		assert.Equal(t, "app1", tags.DDService)
	})
	t.Run("test secondary site actual app3", func(t *testing.T) {
		tags, _ := iisCfg.GetAPMTags("3", "/siteapp2/siteapp3")
		assert.Equal(t, "app3", tags.DDService)
	})
}
