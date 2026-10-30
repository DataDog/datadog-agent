// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
)

func TestParsePackageType(t *testing.T) {
	for raw, expected := range map[string]packages.PackageType{
		"deb": packages.PackageTypeDEB,
		"rpm": packages.PackageTypeRPM,
		"msi": packages.PackageTypeMSI,
		// The macOS .dmg postinstall script runs `installer postinst datadog-agent dmg`.
		"dmg": packages.PackageTypeDMG,
	} {
		t.Run(raw, func(t *testing.T) {
			packageType, err := parsePackageType(raw)
			assert.NoError(t, err)
			assert.Equal(t, expected, packageType)
		})
	}

	_, err := parsePackageType("oci")
	assert.Error(t, err)
}
