// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that addParentPerms does not loop on Windows.
func TestPermissionsInfosAddWindows(t *testing.T) {
	permsInfos := make(permissionsInfos)
	expectedParentPerms := map[string]filePermsInfo{}

	// Basic Case
	path := "C:\\a\\b\\c\\d"
	permsInfos.Add(path)
	assert.NotContains(t, permsInfos, path)
}
