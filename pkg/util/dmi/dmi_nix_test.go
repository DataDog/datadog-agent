// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !serverless

package dmi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSystemSpecificHosttMetadata(t *testing.T) {
	SetupMock(t,
		"UUID1\n",
		"UUID2\n",
		"i-test\n",
		"test vendor\n",
	)

	assert.Equal(t, "UUID1", GetHypervisorUUID())
	assert.Equal(t, "UUID2", GetProductUUID())
	assert.Equal(t, "i-test", GetBoardAssetTag())
	assert.Equal(t, "test vendor", GetBoardVendor())

	hypervisorUUIDPath = "does not exist"
	dmiProductUUIDPath = "does not exist"
	dmiBoardAssetTagPath = "does not exist"
	dmiBoardVendorPath = "does not exist"

	assert.Equal(t, "", GetHypervisorUUID())
	assert.Equal(t, "", GetProductUUID())
	assert.Equal(t, "", GetBoardAssetTag())
	assert.Equal(t, "", GetBoardVendor())
}

func TestGetProductNameAndChassisAssetTag(t *testing.T) {
	SetupMockProductName(t, "Google Compute Engine\n")
	assert.Equal(t, "Google Compute Engine", GetProductName())

	SetupMockChassisAssetTag(t, "7783-7084-3265-9085-8269-3286-77\n")
	assert.Equal(t, "7783-7084-3265-9085-8269-3286-77", GetChassisAssetTag())

	dmiProductNamePath = "does not exist"
	dmiChassisAssetTagPath = "does not exist"

	assert.Equal(t, "", GetProductName())
	assert.Equal(t, "", GetChassisAssetTag())
}
