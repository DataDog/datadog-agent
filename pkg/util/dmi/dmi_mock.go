// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !serverless

package dmi

import (
	"os"
	"path/filepath"
	"testing"
)

func resetSysPath() {
	hypervisorUUIDPath = "/sys/hypervisor/uuid"
	dmiProductUUIDPath = "/sys/devices/virtual/dmi/id/product_uuid"
	dmiBoardAssetTagPath = "/sys/devices/virtual/dmi/id/board_asset_tag"
	dmiBoardVendorPath = "/sys/devices/virtual/dmi/id/board_vendor"
}

func SetupMock(t *testing.T, hypervisorUUID, productUUID, boardAssetTag, boardVendor string) {
	tempDir := t.TempDir()
	t.Cleanup(resetSysPath)

	setTestFile := func(data string, name string) string {
		tempPath := filepath.Join(tempDir, name)
		_ = os.WriteFile(tempPath, []byte(data), os.ModePerm)
		return tempPath
	}

	hypervisorUUIDPath = setTestFile(hypervisorUUID, "hypervisor_uuid")
	dmiProductUUIDPath = setTestFile(productUUID, "product_uuid")
	dmiBoardAssetTagPath = setTestFile(boardAssetTag, "board_asset_tag")
	dmiBoardVendorPath = setTestFile(boardVendor, "board_vendor")
}
