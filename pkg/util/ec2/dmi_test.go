// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/stretchr/testify/assert"
)

func TestIsBoardVendorEC2(t *testing.T) {
	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", true)

	setupDMIForNotEC2(t)
	assert.False(t, isBoardVendorEC2())

	setupDMIForEC2(t)
	assert.True(t, isBoardVendorEC2())

	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", false)
	assert.False(t, isBoardVendorEC2())
}

func TestGetInstanceIDFromDMI(t *testing.T) {
	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", true)

	setupDMIForNotEC2(t)
	instanceID, err := getInstanceIDFromDMI()
	assert.Error(t, err)
	assert.Equal(t, "", instanceID)

	setupDMIForEC2(t)
	instanceID, err = getInstanceIDFromDMI()
	assert.NoError(t, err)
	assert.Equal(t, "i-myinstance", instanceID)

	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", false)
	_, err = getInstanceIDFromDMI()
	assert.Error(t, err)
}

func TestIsEC2UUID(t *testing.T) {
	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", true)

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

func TestIsEC2UUIDSwapEndian(t *testing.T) {
	config.Mock(t)
	config.Datadog().SetWithoutSource("ec2_use_dmi", true)

	// hypervisor
	dmi.SetupMock(t, "45E12AEC-DCD1-B213-94ED-012345ABCDEF", "", "", "")
	assert.True(t, isEC2UUID())

	// product_uuid
	dmi.SetupMock(t, "", "45E12AEC-DCD1-B213-94ED-012345ABCDEF", "", "")
	assert.True(t, isEC2UUID())
}
