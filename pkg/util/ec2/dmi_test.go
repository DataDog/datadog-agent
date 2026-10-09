// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

func TestIsBoardVendorEC2(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)

	setupDMIForNotEC2(t)
	assert.False(t, isBoardVendorEC2())

	setupDMIForEC2(t)
	assert.True(t, isBoardVendorEC2())

	cfg = configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", false)
	assert.False(t, isBoardVendorEC2())
}

func TestGetInstanceIDFromDMI(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)

	setupDMIForNotEC2(t)
	instanceID, err := getInstanceIDFromDMI()
	assert.Error(t, err)
	assert.Equal(t, "", instanceID)

	setupDMIForEC2(t)
	instanceID, err = getInstanceIDFromDMI()
	assert.NoError(t, err)
	assert.Equal(t, "i-myinstance", instanceID)

	cfg = configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", false)
	_, err = getInstanceIDFromDMI()
	assert.Error(t, err)
}

func TestIsEC2UUID(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)

	// no UUID
	dmi.SetupMock(t, "", "", "", "")
	assert.False(t, isEC2UUID())

	// hypervisor
	dmi.SetupMock(t, "ec20b498-1488-4e75-82ba-a6931a9daf36", "", "", "")
	assert.True(t, isEC2UUID())
	dmi.SetupMock(t, "8550b498-1488-4e75-82ba-a6931a9daf36", "", "", "")
	assert.False(t, isEC2UUID())

	// product_uuid
	dmi.SetupMock(t, "", "ec20b498-1488-4e75-82ba-a6931a9daf36", "", "")
	assert.True(t, isEC2UUID())
	dmi.SetupMock(t, "", "8550b498-1488-4e75-82ba-a6931a9daf36", "", "")
	assert.False(t, isEC2UUID())

	// product_uuid with other board vendor
	dmi.SetupMock(t, "", "ec20b498-1488-4e75-82ba-a6931a9daf36", "", "not AWS")
	assert.False(t, isEC2UUID())
	dmi.SetupMock(t, "", "ec20b498-1488-4e75-82ba-a6931a9daf36", "", DMIBoardVendor)
	assert.True(t, isEC2UUID())
}

func TestIsRunningOnDMI(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)

	setupDMIForNotEC2(t)
	assert.False(t, IsRunningOnDMI())

	// board vendor identifies the host as EC2
	setupDMIForEC2(t)
	assert.True(t, IsRunningOnDMI())

	// no board vendor, but UUID identifies the host as EC2
	dmi.SetupMock(t, "ec20b498-1488-4e75-82ba-a6931a9daf36", "", "", "")
	assert.True(t, IsRunningOnDMI())

	dmi.SetupMock(t, "", "", "", "")
	assert.False(t, IsRunningOnDMI())

	cfg = configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", false)
	setupDMIForEC2(t)
	assert.False(t, IsRunningOnDMI())
}

func TestIsRunningOnDMIMetadataSource(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)
	t.Cleanup(resetPackageVars)

	tests := []struct {
		name           string
		hypervisorUUID string
		boardAssetTag  string
		boardVendor    string
		detected       bool
		expectedSource string
	}{
		{"board vendor with instance ID asset tag", "ec2something", "i-myinstance", DMIBoardVendor, true, "DMI"},
		{"board vendor with EC2 UUID", "ec20b498-1488-4e75-82ba-a6931a9daf36", "", DMIBoardVendor, true, "UUID"},
		// detected as EC2, but DMI provides neither an instance ID nor an EC2 UUID, so no source is recorded
		{"board vendor only", "8550b498-1488-4e75-82ba-a6931a9daf36", "", DMIBoardVendor, true, ""},
		{"EC2 UUID only", "ec20b498-1488-4e75-82ba-a6931a9daf36", "", "", true, "UUID"},
		{"not EC2", "", "", "", false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ec2internal.CurrentMetadataSource = ec2internal.MetadataSourceNone
			dmi.SetupMock(t, tc.hypervisorUUID, "", tc.boardAssetTag, tc.boardVendor)

			assert.Equal(t, tc.detected, IsRunningOnDMI())
			assert.Equal(t, tc.expectedSource, ec2internal.GetSourceName())
		})
	}
}

func TestIsEC2UUIDSwapEndian(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("ec2_use_dmi", true)

	// hypervisor
	dmi.SetupMock(t, "45E12AEC-DCD1-B213-94ED-012345ABCDEF", "", "", "")
	assert.True(t, isEC2UUID())

	// product_uuid
	dmi.SetupMock(t, "", "45E12AEC-DCD1-B213-94ED-012345ABCDEF", "", "")
	assert.True(t, isEC2UUID())
}
