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

func TestGetApplicationPath(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	// update global iisCfgPath for test
	iisCfgPath = filepath.Join(path, "testdata", "iisconfig_subapps.xml")
	iisCfg, err := NewDynamicIISConfig()
	require.Nil(t, err)
	require.NotNil(t, iisCfg)

	err = iisCfg.Start()
	require.Nil(t, err)
	defer iisCfg.Stop()

	// Test 1: URL path matching sub-application /api/web2 on site 2
	result := iisCfg.GetApplicationPath(2, "/api/web2")
	assert.Equal(t, "/api/web2", result)

	// Test 2: URL path matches subsite but not as complete path
	result = iisCfg.GetApplicationPath(2, "/api/web2/web3")
	assert.Equal(t, "/api/web2", result)

	// Test 2: URL path that doesn't exist - should fall back to root application
	result = iisCfg.GetApplicationPath(2, "/doesnot/exist")
	assert.Equal(t, "/", result)

	// Test 1: URL path matching sub-application /api/web2 on site 2 when access it using upper case
	result = iisCfg.GetApplicationPath(2, "/api/WEB2")
	assert.Equal(t, "/api/web2", result)

	// Test: Matching is based on URL prefix, not substrings - /api/web2 in the middle of the path should not match the /api/web2 application
	result = iisCfg.GetApplicationPath(2, "/app/TestSite/api/web2")
	assert.Equal(t, "/", result)

	// Test: "/api/web" should return "/" because it doesn't fully match "/api/web1" or "/api/web2"
	result = iisCfg.GetApplicationPath(2, "/api/web")
	assert.Equal(t, "/", result)
}
