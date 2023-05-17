// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || serverless

package dmi

// Used by the mock
var (
	boardAssetTag  = ""
	boardVendor    = ""
	productUUID    = ""
	hypervisorUUID = ""
)

// GetBoardAssetTag returns an empty string on Windows
func GetBoardAssetTag() string {
	return boardAssetTag
}

// GetBoardVendor returns an empty string on Windows
func GetBoardVendor() string {
	return boardVendor
}

// GetProductUUID returns an empty string on Windows
func GetProductUUID() string {
	return productUUID
}

// GetHypervisorUUID returns an empty string on Windows
func GetHypervisorUUID() string {
	return hypervisorUUID
}
