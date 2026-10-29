// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnumDriverServices verifies against the real host that the SCM enumeration finds every
// kernel-mode and file-system driver service, with an image path for most of them.
//
// "disk" is checked by name because every Windows host has it: it is the storage class driver
// and is present from the very first boot.
func TestEnumDriverServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	drivers, err := EnumDriverServices()
	require.NoError(t, err)
	require.NotEmpty(t, drivers, "a Windows host always has kernel driver services")

	var withImagePath int
	var foundDisk bool
	for _, driver := range drivers {
		assert.NotEmpty(t, driver.Name, "every service record has a name")
		if driver.ImagePath != "" {
			withImagePath++
		}
		if strings.EqualFold(driver.Name, "disk") {
			foundDisk = true
		}
	}

	assert.True(t, foundDisk, "the disk class driver is present on every Windows host")

	minimumWithImagePath := len(drivers) * 9 / 10
	assert.GreaterOrEqual(t, withImagePath, minimumWithImagePath,
		"at least 90%% of driver services should have a resolvable image path, got %d/%d",
		withImagePath, len(drivers))
}

func TestResolveDriverImagePath(t *testing.T) {
	const windowsDir = `C:\Windows`

	t.Setenv("SystemRoot", windowsDir)

	tests := []struct {
		name     string
		pathName string
		expected string
	}{
		{
			name:     "already rooted at a drive letter is returned untouched",
			pathName: `C:\Program Files\Vendor\driver.sys`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "a quoted path is unquoted",
			pathName: `"C:\Program Files\Vendor\driver.sys"`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "arguments after a quoted path are ignored",
			pathName: `"%SystemRoot%\System32\drivers\driver.sys" -flag`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "a UNC path is returned untouched",
			pathName: `\\fileserver\share\driver.sys`,
			expected: `\\fileserver\share\driver.sys`,
		},
		{
			name:     "forward slashes after a drive letter are left alone",
			pathName: `C:/Windows/System32/drivers/driver.sys`,
			expected: `C:/Windows/System32/drivers/driver.sys`,
		},
		{
			name:     "the NT object-manager prefix is stripped",
			pathName: `\??\C:\Program Files\Vendor\driver.sys`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "a quoted NT object-manager path is unquoted and stripped",
			pathName: `"\??\C:\Program Files\Vendor\driver.sys"`,
			expected: `C:\Program Files\Vendor\driver.sys`,
		},
		{
			name:     "SystemRoot resolves against the Windows directory",
			pathName: `\SystemRoot\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "SystemRoot is matched case-insensitively",
			pathName: `\systemroot\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "a bare relative path resolves against the Windows directory",
			pathName: `System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "a leading separator is not a root",
			pathName: `\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "environment variables are expanded",
			pathName: `%SystemRoot%\System32\drivers\driver.sys`,
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
		{
			name:     "surrounding whitespace is ignored",
			pathName: "  " + `C:\Windows\System32\drivers\driver.sys` + "  ",
			expected: `C:\Windows\System32\drivers\driver.sys`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveDriverImagePath(test.pathName, windowsDir)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveDriverImagePathRejectsEmptyPaths(t *testing.T) {
	for _, pathName := range []string{"", "   ", "\t"} {
		_, err := ResolveDriverImagePath(pathName, `C:\Windows`)
		assert.Error(t, err, "an empty image path is not resolvable")
	}
}

func TestResolveDriverImagePathRejectsMalformedQuotedPaths(t *testing.T) {
	for _, pathName := range []string{`"`, `"C:\Program Files\Vendor\driver.sys`} {
		_, err := ResolveDriverImagePath(pathName, `C:\Windows`)
		assert.Error(t, err, "an unterminated quoted image path is not resolvable")
	}
}
