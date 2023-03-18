// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || serverless

package dmi

import (
	"testing"
)

func SetupMock(t *testing.T, testHypervisorUUID, testProductUUID, testBoardAssetTag, testBoardVendor string) {
	t.Cleanup(func() {
		boardAssetTag = ""
		boardVendor = ""
		productUUID = ""
		hypervisorUUID = ""
	})

	boardAssetTag = testBoardAssetTag
	boardVendor = testBoardVendor
	productUUID = testProductUUID
	hypervisorUUID = testHypervisorUUID
}
