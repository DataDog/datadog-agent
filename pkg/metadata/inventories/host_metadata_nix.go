// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package inventories

import (
	"os"
	"strings"
)

var (
	hypervisorUUIDPath   = "/sys/hypervisor/uuid"
	dmiProductUUIDPath   = "/sys/devices/virtual/dmi/id/product_uuid"
	dmiBoardAssetTagPath = "/sys/devices/virtual/dmi/id/board_asset_tag"
	dmiBoardVendorPath   = "/sys/devices/virtual/dmi/id/board_vendor"
)

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(data), "\n")
}

func getSystemSpecificHosttMetadata(metadata *HostMetadata) {
	metadata.HypervisorGuestUUID = readFile(hypervisorUUIDPath)
	metadata.DmiProductUUID = readFile(dmiProductUUIDPath)
	metadata.DmiBoardAssetTag = readFile(dmiBoardAssetTagPath)
	metadata.DmiBoardVendor = readFile(dmiBoardVendorPath)
}
