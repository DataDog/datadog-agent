// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
)

// TestParsePackageTypeAcceptsEveryDeclaredType pins the CLI's accepted set against the enum. A
// package type the enum declares but the parser rejects is a package that cannot run its own
// hooks, which is only discoverable by installing it.
func TestParsePackageTypeAcceptsEveryDeclaredType(t *testing.T) {
	for _, packageType := range []packages.PackageType{
		packages.PackageTypeMSI,
		packages.PackageTypeDEB,
		packages.PackageTypeRPM,
		packages.PackageTypeDMG,
	} {
		parsed, err := parsePackageType(string(packageType))
		require.NoError(t, err, "the CLI rejects the declared package type %s", packageType)
		assert.Equal(t, packageType, parsed)
	}
}

// TestParsePackageTypeRejectsUnknownValues is the other half: widening the parser for dmg must
// not have widened it for everything.
func TestParsePackageTypeRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"", "pkg", "DMG", "dmg ", "oci", "tar"} {
		_, err := parsePackageType(raw)
		assert.Error(t, err, "the CLI accepted %q as a package type", raw)
	}
}

// TestPostinstPackagePathIsFixedForLinuxPackages covers the path the deb and rpm scripts take,
// which must keep naming the one fixed root regardless of where the installer runs from.
func TestPostinstPackagePathIsFixedForLinuxPackages(t *testing.T) {
	for _, packageType := range []packages.PackageType{packages.PackageTypeDEB, packages.PackageTypeRPM, packages.PackageTypeMSI} {
		path, err := postinstPackagePath(packageType)
		require.NoError(t, err)
		assert.Equal(t, "/opt/datadog-agent", path)
	}
}

// TestPostinstPackagePathRejectsADmgOutsideThePool is the guard that matters: the dmg path is
// derived from the running binary's location, so an installer invoked with the dmg type from
// anywhere but the pool must fail loudly rather than hand a hook a wrong code root. The test
// binary is never in the pool, so this is the case it exercises.
func TestPostinstPackagePathRejectsADmgOutsideThePool(t *testing.T) {
	_, err := postinstPackagePath(packages.PackageTypeDMG)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "datadog-agent")
}
