// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

// IANA information elements
const (
	// application table
	ipfixFieldApplicationDescription uint16 = 94
	ipfixFieldApplicationID          uint16 = 95
	ipfixFieldApplicationName        uint16 = 96

	// application attributes
	ipfixFieldP2PTechnology       uint16 = 288
	ipfixFieldTunnelTechnology    uint16 = 289
	ipfixFieldEncryptedTechnology uint16 = 290
)

// Cisco enterprise-specific fields (PEN 9), sent alongside IANA fields in NBAR's `option application-attributes` records
const (
	ciscoPEN                               uint32 = 9
	ciscoFieldApplicationFamily            uint16 = 12230
	ciscoFieldApplicationSet               uint16 = 12231
	ciscoFieldApplicationCategory          uint16 = 12232
	ciscoFieldApplicationSubCategory       uint16 = 12233
	ciscoFieldApplicationGroup             uint16 = 12234
	ciscoFieldApplicationTrafficClass      uint16 = 12243
	ciscoFieldApplicationBusinessRelevance uint16 = 12244
)
