// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build windows

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddParentPermsWindows(t *testing.T) {
	assert := assert.New(t)

	permsInfos := make(permissionsInfos)

	// Basic Case
	path := "C:\\a\\b\\c\\d"
	addParentPerms(path, permsInfos)
	expectedParentPerms := map[string]filePermsInfo{
		"/": {0, "", ""}, "/a": {0, "", ""}, "/a/b": {0, "", ""}, "/a/b/c": {0, "", ""},
	}
	assert.EqualValues(permsInfos, expectedParentPerms)
	assert.Equal(true, false)

	// Empty Case
	permsInfos = make(permissionsInfos)
	path = ""
	addParentPerms(path, permsInfos)
	expectedParentPerms = map[string]filePermsInfo{}
	assert.EqualValues(permsInfos, expectedParentPerms)

	// Only root
	permsInfos = make(permissionsInfos)
	path = "/"
	addParentPerms(path, permsInfos)
	expectedParentPerms = map[string]filePermsInfo{}
	assert.EqualValues(permsInfos, expectedParentPerms)

	// Space in path
	permsInfos = make(permissionsInfos)
	path = "/a b/c"
	addParentPerms(path, permsInfos)
	expectedParentPerms = map[string]filePermsInfo{
		"/": {0, "", ""}, "/a b": {0, "", ""},
	}
	assert.EqualValues(permsInfos, expectedParentPerms)

	// Dot in path
	permsInfos = make(permissionsInfos)
	path = "/a.b/c"
	addParentPerms(path, permsInfos)
	expectedParentPerms = map[string]filePermsInfo{
		"/": {0, "", ""}, "/a.b": {0, "", ""},
	}
	assert.EqualValues(permsInfos, expectedParentPerms)

}
