// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && (amd64 || arm64)

package procinfo

// sizeofProcessEntry32 is the byte size of windows.ProcessEntry32 on 64-bit
// Windows. Manually verified from the struct layout:
//
//	uint32×3 + 4-byte alignment padding + uintptr(8) + uint32×5 + [260]uint16
//	= 12 + 4 + 8 + 20 + 520 = 564, rounded up to 8-byte alignment = 568
const sizeofProcessEntry32 = uint32(568)
