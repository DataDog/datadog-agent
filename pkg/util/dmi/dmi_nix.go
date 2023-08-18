// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !serverless

package dmi

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

// GetBoardAssetTag returns the board asset tag from DMI
func GetBoardAssetTag() string {
	return readFile(dmiBoardAssetTagPath)
}

// GetBoardVendor returns the board vendor
func GetBoardVendor() string {
	return readFile(dmiBoardVendorPath)
}

// GetProductUUID returns the product UUID
func GetProductUUID() string {
	return readFile(dmiProductUUIDPath)
}

// GetHypervisorUUID returns the hypervisor UUID
func GetHypervisorUUID() string {
	return readFile(hypervisorUUIDPath)
}
