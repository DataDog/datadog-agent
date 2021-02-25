// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build windows

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that addParentPerms does not loop on Windows.
func TestAddParentPermsWindows(t *testing.T) {
	assert := assert.New(t)

	permsInfos := make(permissionsInfos)
	expectedParentPerms := map[string]filePermsInfo{}

	// Basic Case
	path := "C:\\a\\b\\c\\d"
	addParentPerms(path, permsInfos)
	assert.EqualValues(expectedParentPerms, permsInfos)

	// Empty Case
	path = ""
	addParentPerms(path, permsInfos)
	assert.EqualValues(expectedParentPerms, permsInfos)

	// Only root
	permsInfos = make(permissionsInfos)
	path = "C:\\"
	addParentPerms(path, permsInfos)
	assert.EqualValues(expectedParentPerms, permsInfos)

	// Space in path
	permsInfos = make(permissionsInfos)
	path = "D:\\a b\\c"
	addParentPerms(path, permsInfos)
	assert.EqualValues(expectedParentPerms, permsInfos)

	// Dot in path
	permsInfos = make(permissionsInfos)
	path = "E:\\a.b\\c"
	addParentPerms(path, permsInfos)
	assert.EqualValues(expectedParentPerms, permsInfos)

}
