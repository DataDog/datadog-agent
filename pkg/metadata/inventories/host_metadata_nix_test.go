// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package inventories

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func resetSysPath() {
	hypervisorUUIDPath = "/sys/hypervisor/uuid"
	dmiProductUUIDPath = "/sys/devices/virtual/dmi/id/product_uuid"
	dmiBoardAssetTagPath = "/sys/devices/virtual/dmi/id/board_asset_tag"
	dmiBoardVendorPath = "/sys/devices/virtual/dmi/id/board_vendor"
}

func TestGetSystemSpecificHosttMetadata(t *testing.T) {
	tempDir := t.TempDir()
	defer resetSysPath()

	setTestFile := func(data string, name string) string {
		tempPath := filepath.Join(tempDir, name)
		os.WriteFile(tempPath, []byte(data), os.ModePerm)
		return tempPath
	}

	hypervisorUUIDPath = setTestFile("a404eff4-bacf-4839-a33c-619c3a06abd1\n", "hypervisor_uuid")
	dmiProductUUIDPath = setTestFile("a404eff4-bacf-4839-a33c-619c3a06abd1\n", "product_uuid")
	dmiBoardAssetTagPath = setTestFile("i-test\n", "board_asset_tag")
	dmiBoardVendorPath = setTestFile("test vendor\n", "board_vendor")

	metadata := &HostMetadata{}
	getSystemSpecificHosttMetadata(metadata)
	assert.Equal(t, "a404eff4-bacf-4839-a33c-619c3a06abd1", metadata.HypervisorGuestUUID)
	assert.Equal(t, "a404eff4-bacf-4839-a33c-619c3a06abd1", metadata.DmiProductUUID)
	assert.Equal(t, "i-test", metadata.DmiBoardAssetTag)
	assert.Equal(t, "test vendor", metadata.DmiBoardVendor)

	hypervisorUUIDPath = "does not exist"
	dmiProductUUIDPath = "does not exist"
	dmiBoardAssetTagPath = "does not exist"
	dmiBoardVendorPath = "does not exist"

	metadata = &HostMetadata{}
	getSystemSpecificHosttMetadata(metadata)
	assert.Equal(t, "", metadata.HypervisorGuestUUID)
	assert.Equal(t, "", metadata.DmiProductUUID)
	assert.Equal(t, "", metadata.DmiBoardAssetTag)
	assert.Equal(t, "", metadata.DmiBoardVendor)
}
