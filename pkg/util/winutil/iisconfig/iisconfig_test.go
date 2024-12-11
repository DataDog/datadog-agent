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

func TestConfigLoading(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "iisconfig.xml")
	iisCfg, err := NewDynamicIISConfig()
	assert.Nil(t, err)
	assert.NotNil(t, iisCfg)

	err = iisCfg.Start()
	assert.Nil(t, err)

	name := iisCfg.GetSiteNameFromID(0)
	assert.Equal(t, name, "")
	name = iisCfg.GetSiteNameFromID(1)
	assert.Equal(t, name, "Default Web Site")
	name = iisCfg.GetSiteNameFromID(2)
	assert.Equal(t, name, "TestSite")
	iisCfg.Stop()
}

func TestUninitializedConfig(t *testing.T) {
	path, err := os.Getwd()
	require.Nil(t, err)

	iisCfgPath = filepath.Join(path, "testdata", "iisconfig.xml")
	iisCfg, err := NewDynamicIISConfig()

	// by not calling start, this will simulate either a caller trying to use w/o calling
	// start, or a race where the start is still "in progress" when the caller tries to use it

	assert.Nil(t, err)
	assert.NotNil(t, iisCfg)

	name := iisCfg.GetSiteNameFromID(0)
	assert.Equal(t, name, "")

	atags, cfgtags := iisCfg.GetAPMTags(0, "/")
	assert.Equal(t, atags.DDService, "")
	assert.Equal(t, cfgtags.DDService, "")

}
